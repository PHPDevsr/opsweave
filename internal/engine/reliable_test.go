package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

func reliableEngineFixture(t *testing.T, h action.Handler, document string) (Engine, *sqlite.Store) {
	t.Helper()
	e, s, _ := fixture(t, h)
	e.Reliable = s
	if document != "" {
		catalog, err := e.Actions.Catalog(map[string]string{"test.echo": "1"})
		if err != nil {
			t.Fatal(err)
		}
		candidates, err := registry.PrepareReliable(t.Context(), []registry.Source{{Data: []byte(document)}}, catalog)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(t.Context(), candidates); err != nil {
			t.Fatal(err)
		}
	}
	return e, s
}

func TestReliableCoordinatorRetriesReferencesAndConditions(t *testing.T) {
	document := `apiVersion: opsweave.io/v1
kind: Workflow
metadata: {name: example}
spec:
  steps:
    - id: first
      uses: test.echo
      with: {message: first}
      retry: {attempts: 2, backoff: 1ms, maxBackoff: 1ms}
    - id: second
      uses: test.echo
      with: {message: {$ref: steps.first.outputs.value}}
    - id: third
      uses: test.echo
      if: 'false'
      with: {message: never}
`
	calls := 0
	e, s := reliableEngineFixture(t, func(ctx context.Context, input map[string]any) (map[string]any, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("private-action-detail")
		}
		if calls == 3 && input["message"] != "first-output" {
			t.Error(input)
		}
		return map[string]any{"value": input["message"].(string) + "-output", "secret": "redacted"}, nil
	}, document)
	snap, err := e.Run(t.Context(), "example")
	if err != nil || snap.State != execution.Succeeded || snap.Format != 2 || calls != 3 || len(snap.Steps[0].Attempts) != 2 || snap.Steps[2].State != execution.Skipped {
		t.Fatal(snap, err, calls)
	}
	stored, err := s.Execution(t.Context(), snap.ID)
	if err != nil || stored.Steps[0].Attempts[0].State != execution.Failed {
		t.Fatal(stored, err)
	}
	events, err := s.Events(t.Context(), snap.ID)
	data, encodeErr := json.Marshal(events)
	if err != nil || encodeErr != nil || strings.Contains(string(data), "private-action-detail") || strings.Contains(string(data), "redacted") {
		t.Fatal(err, encodeErr)
	}
}

func TestReliableCoordinatorExhaustionAndFailureHandler(t *testing.T) {
	document := strings.Replace(source, "with: {message: hello}}", "with: {message: hello}, retry: {attempts: 2, backoff: 1ms, maxBackoff: 1ms}}", 1)
	document = strings.Replace(document, "- {id: second, uses: test.echo, with: {message: world}}", "- {id: second, uses: test.echo, if: steps.first.failed, with: {message: world}}", 1)
	calls := 0
	e, _ := reliableEngineFixture(t, func(_ context.Context, in map[string]any) (map[string]any, error) {
		calls++
		if in["message"] == "hello" {
			return nil, errors.New("failed")
		}
		return map[string]any{}, nil
	}, document)
	snap, err := e.Run(t.Context(), "example")
	if err != execution.ErrAction || snap.State != execution.Failed || snap.Steps[1].State != execution.Succeeded || calls != 3 {
		t.Fatal(snap, err, calls)
	}
}

type blockedClock struct {
	entered chan struct{}
	once    sync.Once
}

func (c *blockedClock) WaitUntil(ctx context.Context, _ time.Time) error {
	c.once.Do(func() { close(c.entered) })
	<-ctx.Done()
	return ctx.Err()
}

type engineResult struct {
	snap execution.Snapshot
	err  error
}

func TestReliableCoordinatorDurableCancellationDuringBackoff(t *testing.T) {
	for _, whole := range []bool{false, true} {
		t.Run(map[bool]string{false: "step", true: "execution"}[whole], func(t *testing.T) {
			document := strings.Replace(source, "with: {message: hello}}", "with: {message: hello}, retry: {attempts: 2, backoff: 1m, maxBackoff: 1m}}", 1)
			calls := 0
			e, s := reliableEngineFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
				calls++
				return nil, errors.New("retry me")
			}, document)
			clock := &blockedClock{entered: make(chan struct{})}
			e.Clock = clock
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			var wg sync.WaitGroup
			defer func() { cancel(); wg.Wait() }()
			result := make(chan engineResult, 1)
			wg.Go(func() { snap, err := e.Run(ctx, "example"); result <- engineResult{snap, err} })
			select {
			case <-clock.entered:
			case <-ctx.Done():
				t.Fatal("backoff not reached")
			}
			list, err := s.Executions(t.Context())
			if err != nil || len(list) != 1 {
				t.Fatal(list, err)
			}
			step := "first"
			if whole {
				step = ""
			}
			acknowledged, err := s.RequestReliableCancellation(t.Context(), list[0].ID, step)
			if err != nil || acknowledged.State != execution.Running {
				t.Fatal(acknowledged, err)
			}
			select {
			case got := <-result:
				if got.err != context.Canceled || got.snap.State != execution.Cancelled || calls != 1 || len(got.snap.Steps[0].Attempts) != 1 {
					t.Fatal(got, calls)
				}
				want := execution.Skipped
				if whole {
					want = execution.Cancelled
				}
				if got.snap.Steps[1].State != want {
					t.Fatal(got.snap.Steps)
				}
			case <-ctx.Done():
				t.Fatal("cancellation did not join owned work")
			}
		})
	}
}

func TestReliableCoordinatorConcurrentAdmissionDoesNotRepeatActions(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	e, _ := reliableEngineFixture(t, func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		calls.Add(1)
		once.Do(func() { close(entered) })
		select {
		case <-release:
			return map[string]any{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, "")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	results := make(chan engineResult, 2)
	wg.Go(func() {
		s, err := e.RunReliable(ctx, "example", RunOptions{IdempotencyKey: "same"})
		results <- engineResult{s, err}
	})
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	wg.Go(func() {
		s, err := e.RunReliable(ctx, "example", RunOptions{IdempotencyKey: "same"})
		results <- engineResult{s, err}
	})
	close(release)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.snap.ID != second.snap.ID || calls.Load() != 2 {
		t.Fatal(first, second, calls.Load())
	}
}

type reliableFault struct {
	execution.ReliableRepository
	at        int
	calls     int
	cause     error
	control   bool
	ambiguous bool
}

func (r *reliableFault) ChangeReliable(ctx context.Context, m execution.ReliableMutation) (execution.Snapshot, error) {
	r.calls++
	if r.calls == r.at {
		if r.ambiguous {
			snap, err := r.ReliableRepository.ChangeReliable(ctx, m)
			if err != nil {
				return snap, err
			}
			return snap, r.cause
		}
		return execution.Snapshot{}, r.cause
	}
	return r.ReliableRepository.ChangeReliable(ctx, m)
}
func (r *reliableFault) ReliableControl(ctx context.Context, id, owner string) (execution.Cancellation, error) {
	if r.control {
		return execution.Cancellation{}, r.cause
	}
	return r.ReliableRepository.ReliableControl(ctx, id, owner)
}

func TestReliableCoordinatorPersistenceAndAmbiguousFailures(t *testing.T) {
	for _, at := range []int{1, 2, 3, 7} {
		t.Run(string(rune('0'+at)), func(t *testing.T) {
			calls := 0
			e, s := reliableEngineFixture(t, func(context.Context, map[string]any) (map[string]any, error) { calls++; return map[string]any{}, nil }, "")
			cause := errors.New("private-database-detail")
			e.Reliable = &reliableFault{ReliableRepository: s, at: at, cause: cause, ambiguous: at == 2}
			snap, err := e.Run(t.Context(), "example")
			var failure *execution.Failure
			if !errors.As(err, &failure) || !errors.Is(err, cause) || strings.Contains(err.Error(), "private-database-detail") {
				t.Fatal(err)
			}
			want := 0
			if at == 3 {
				want = 1
			}
			if at == 7 {
				want = 2
			}
			if calls != want {
				t.Fatal(calls, want)
			}
			stored, err := s.Execution(t.Context(), snap.ID)
			if err != nil || stored.State != execution.Running {
				t.Fatal(stored, err)
			}
			if _, err := e.Run(t.Context(), "example"); err != execution.ErrConflict {
				t.Fatal("unsafe resubmission", err)
			}
		})
	}
}

func TestReliableCoordinatorControlFailureCancelsAction(t *testing.T) {
	e, s := reliableEngineFixture(t, func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, "")
	cause := errors.New("private-control-detail")
	e.Reliable = &reliableFault{ReliableRepository: s, cause: cause, control: true}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	snap, err := e.Run(ctx, "example")
	var failure *execution.Failure
	if !errors.As(err, &failure) || !errors.Is(err, cause) {
		t.Fatal(err)
	}
	stored, err := s.Execution(t.Context(), snap.ID)
	if err != nil || stored.State != execution.Running {
		t.Fatal(stored, err)
	}
}

func TestReliableCoordinatorReplayPinsRevision(t *testing.T) {
	fail := true
	e, s := reliableEngineFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
		if fail {
			return nil, errors.New("failed")
		}
		return map[string]any{}, nil
	}, "")
	original, err := e.Run(t.Context(), "example")
	if err != execution.ErrAction {
		t.Fatal(err)
	}
	catalog, err := e.Actions.Catalog(map[string]string{"test.echo": "1"})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := registry.PrepareReliable(t.Context(), []registry.Source{{Data: []byte(strings.Replace(source, "hello", "new", 1))}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(t.Context(), candidates); err != nil {
		t.Fatal(err)
	}
	fail = false
	replayed, err := e.Retry(t.Context(), original.ID, RunOptions{IdempotencyKey: "retry-key"})
	if err != nil || replayed.ID == original.ID || replayed.Revision != original.Revision || replayed.Parent != original.ID {
		t.Fatal(replayed, err)
	}
	unchanged, err := s.Execution(t.Context(), original.ID)
	if err != nil || unchanged.State != execution.Failed {
		t.Fatal(unchanged, err)
	}
	if _, err := e.Retry(t.Context(), replayed.ID, RunOptions{}); err != execution.ErrInvalid {
		t.Fatal("replayed succeeded history", err)
	}
}

func TestReliableCoordinatorMissingOutputDoesNotInvokeDependent(t *testing.T) {
	document := strings.Replace(source, "with: {message: world}", "with: {message: {$ref: steps.first.outputs.value}}", 1)
	calls := 0
	e, _ := reliableEngineFixture(t, func(context.Context, map[string]any) (map[string]any, error) { calls++; return map[string]any{}, nil }, document)
	snap, err := e.Run(t.Context(), "example")
	if err != execution.ErrAction || calls != 1 || snap.Steps[1].Reason != "reference_unavailable" || len(snap.Steps[1].Attempts) != 0 {
		t.Fatal(snap, err, calls)
	}
}

func TestReliableCoordinatorIndependentRootsAndAllSkipped(t *testing.T) {
	for _, skip := range []bool{false, true} {
		doc := strings.Replace(source, "id: second, uses: test.echo", "id: second, needs: [], uses: test.echo", 1)
		if skip {
			doc = strings.ReplaceAll(doc, "uses: test.echo", "uses: test.echo, if: 'false'")
		}
		calls := 0
		e, _ := reliableEngineFixture(t, func(_ context.Context, input map[string]any) (map[string]any, error) {
			calls++
			if input["message"] == "hello" {
				return nil, errors.New("failed root")
			}
			return map[string]any{}, nil
		}, doc)
		snap, err := e.Run(t.Context(), "example")
		if skip {
			if err != nil || snap.State != execution.Succeeded || calls != 0 {
				t.Fatal(snap, err, calls)
			}
		} else {
			if err != execution.ErrAction || calls != 2 || snap.Steps[1].State != execution.Succeeded {
				t.Fatal(snap, err, calls)
			}
		}
	}
}

func TestReliableCoordinatorStepDeadlineIncludesBackoff(t *testing.T) {
	doc := strings.Replace(source, "with: {message: hello}}", "timeout: 5s, with: {message: hello}, retry: {attempts: 2, backoff: 1m, maxBackoff: 1m}}", 1)
	calls := 0
	e, _ := reliableEngineFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
		calls++
		return nil, errors.New("retryable")
	}, doc)
	clock := &blockedClock{entered: make(chan struct{})}
	e.Clock = clock
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	snap, err := e.Run(ctx, "example")
	if err != execution.ErrAction || snap.State != execution.Failed || snap.Steps[0].State != execution.TimedOut || len(snap.Steps[0].Attempts) != 1 || calls != 1 {
		t.Fatal(snap, err, calls)
	}
	select {
	case <-clock.entered:
	default:
		t.Fatal("backoff was not measured inside the step deadline")
	}
}

type earlyDeadlineClock struct{ early bool }

func (c *earlyDeadlineClock) WaitUntil(_ context.Context, at time.Time) error {
	c.early = time.Now().Before(at)
	return context.DeadlineExceeded
}

func TestReliableCoordinatorEarlyDeadlineCannotStartRetry(t *testing.T) {
	doc := strings.Replace(source, "with: {message: hello}}", "timeout: 1s, with: {message: hello}, retry: {attempts: 2, backoff: 1m, maxBackoff: 1m}}", 1)
	calls := 0
	e, _ := reliableEngineFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
		calls++
		return nil, errors.New("retryable")
	}, doc)
	clock := &earlyDeadlineClock{}
	e.Clock = clock
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	snap, err := e.Run(ctx, "example")
	if !clock.early || err != execution.ErrAction || snap.State != execution.Failed || snap.Steps[0].State != execution.TimedOut || len(snap.Steps[0].Attempts) != 1 || calls != 1 {
		t.Fatal("early timer violated durable retry boundary", snap, err, calls, clock.early)
	}
}

func TestSystemClockWaitUntilChecksDueTimeAndCancellation(t *testing.T) {
	at := time.Now().Add(20 * time.Millisecond)
	if err := (systemClock{}).WaitUntil(t.Context(), at); err != nil || time.Now().Before(at) {
		t.Fatal("wait returned before due time", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (systemClock{}).WaitUntil(ctx, time.Now().Add(time.Hour)); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestReliableCoordinatorExecutionDeadlineAndCallerCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		var cancel context.CancelFunc
		doc := source
		if deadline {
			doc = strings.Replace(doc, "spec:\n", "spec:\n  timeout: 5s\n", 1)
		}
		e, _ := reliableEngineFixture(t, func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			if !deadline {
				cancel()
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}, doc)
		ctx, stop := context.WithTimeout(t.Context(), 20*time.Second)
		defer stop()
		ctx, cancel = context.WithCancel(ctx)
		snap, err := e.Run(ctx, "example")
		cancel()
		want := execution.Cancelled
		cause := context.Canceled
		if deadline {
			want = execution.TimedOut
			cause = context.DeadlineExceeded
		}
		if err != cause || snap.State != want || snap.Steps[1].State != execution.Cancelled {
			t.Fatal(snap, err)
		}
	}
}

func TestReliableCoordinatorCancelsMaximumGraphWithinCleanupBudget(t *testing.T) {
	var document strings.Builder
	document.WriteString("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n")
	for i := range 256 {
		fmt.Fprintf(&document, "    - {id: step-%d, uses: test.echo, with: {message: hello}}\n", i)
	}
	var cancel context.CancelFunc
	calls := 0
	e, _ := reliableEngineFixture(t, func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		calls++
		cancel()
		return nil, ctx.Err()
	}, document.String())
	ctx, stop := context.WithTimeout(t.Context(), 30*time.Second)
	defer stop()
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	snap, err := e.Run(ctx, "example")
	if err != context.Canceled || snap.State != execution.Cancelled || calls != 1 || len(snap.Steps) != 256 {
		t.Fatal(snap.State, err, calls)
	}
	for _, step := range snap.Steps {
		if step.State != execution.Cancelled {
			t.Fatal(step.ID, step.State)
		}
	}
}

func TestReliableCoordinatorPreflightRejectsCorruption(t *testing.T) {
	for _, edit := range []func(*registry.Record){func(r *registry.Record) { r.Bindings = json.RawMessage(`invalid`) }, func(r *registry.Record) { r.Bindings = json.RawMessage(`{"test.echo":"missing"}`) }, func(r *registry.Record) { r.Source = []byte("invalid") }, func(r *registry.Record) { r.Definition = json.RawMessage(`{}`) }} {
		e, s := reliableEngineFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
			t.Fatal("invoked corrupt definition")
			return nil, nil
		}, "")
		e.Workflows = alteredWorkflows{Repository: s, edit: edit}
		if _, err := e.Run(t.Context(), "example"); err == nil {
			t.Fatal("corruption accepted")
		}
		if list, err := s.Executions(t.Context()); err != nil || len(list) != 0 {
			t.Fatal(list, err)
		}
	}
	if _, err := (Engine{}).RunReliable(t.Context(), "example", RunOptions{}); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := (Engine{}).Retry(t.Context(), "invalid", RunOptions{}); err != execution.ErrInvalid {
		t.Fatal(err)
	}
}

func TestReliableReceiptWaitIsBoundedWithoutTakingOwnership(t *testing.T) {
	deadline := time.Now().Add(-time.Second)
	snapshot := execution.Snapshot{State: execution.Running, Deadline: &deadline}
	got, err := (Engine{}).waitReceipt(t.Context(), snapshot)
	if err != context.DeadlineExceeded || got.State != execution.Running {
		t.Fatal(got, err)
	}
	snapshot.Deadline = nil
	if _, err := (Engine{}).waitReceipt(t.Context(), snapshot); err != execution.ErrInvalid {
		t.Fatal(err)
	}
}

package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/dispatchlock"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

const parallelSource = `apiVersion: opsweave.io/v1
kind: Workflow
metadata: {name: example}
spec:
  parallelism: 2
  concurrency: {limit: 2}
  steps:
    - {id: first, uses: test.echo, needs: [], with: {message: first}}
    - {id: second, uses: test.echo, needs: [], with: {message: second}}
    - {id: third, uses: test.echo, needs: [first, second], with: {message: {$ref: steps.first.outputs.value}}}
`

func dispatchFixture(t *testing.T, h action.Handler, document string) (Dispatcher, *sqlite.Store) {
	t.Helper()
	e, s, path := fixture(t, h)
	catalog, err := e.Actions.Catalog(map[string]string{"test.echo": "1"})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := registry.PrepareDispatcher(t.Context(), []registry.Source{{Data: []byte(document)}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(t.Context(), candidates); err != nil {
		t.Fatal(err)
	}
	lock, err := dispatchlock.Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	})
	fence, err := s.BeginDispatcher(t.Context(), strings.Repeat("a", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{Engine: e, Repository: s, Fence: fence, Capacity: execution.DefaultCapacity()}, s
}

func TestDispatcherResolutionAndFailureHandler(t *testing.T) {
	for _, scenario := range []string{"missing_output", "retry_exhaustion"} {
		t.Run(scenario, func(t *testing.T) {
			doc := strings.Replace(source, "with: {message: world}", "with: {message: {$ref: steps.first.outputs.value}}", 1)
			if scenario == "retry_exhaustion" {
				doc = strings.Replace(source, "with: {message: hello}", "retry: {attempts: 2, backoff: 1ms, maxBackoff: 1ms}, with: {message: hello}", 1)
				doc = strings.Replace(doc, "with: {message: world}", "if: 'steps.first.failed', with: {message: handler}", 1)
			}
			calls := 0
			d, s := dispatchFixture(t, func(_ context.Context, in map[string]any) (map[string]any, error) {
				calls++
				if scenario == "retry_exhaustion" && in["message"] != "handler" {
					return nil, errors.New("private action error")
				}
				return map[string]any{}, nil
			}, doc)
			q, err := d.Engine.Enqueue(t.Context(), s, d.Capacity, "example", RunOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got, err := d.Run(t.Context(), q.Snapshot.ID)
			if !errors.Is(err, execution.ErrAction) || got.State != execution.Failed {
				t.Fatal(got, err, errors.Unwrap(err))
			}
			if scenario == "missing_output" {
				if calls != 1 || got.Steps[1].Reason != "reference_unavailable" || len(got.Steps[1].Attempts) != 0 {
					t.Fatal(got, calls)
				}
			} else {
				if calls != 3 || got.Steps[1].State != execution.Succeeded || len(got.Steps[0].Attempts) != 2 {
					t.Fatal(got, calls)
				}
			}
		})
	}
}

func TestDispatcherRejectsUnavailableQueuedActionsAndCancelledAdmission(t *testing.T) {
	for _, scenario := range []string{"unavailable", "caller_cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			d, s := dispatchFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
				calls.Add(1)
				return map[string]any{}, nil
			}, source)
			q, err := d.Engine.Enqueue(t.Context(), s, d.Capacity, "example", RunOptions{})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if scenario == "unavailable" {
				d.Engine.Actions = nil
			} else {
				cancel()
			}
			snap, err := d.Run(ctx, q.Snapshot.ID)
			if !errors.Is(err, context.Canceled) || snap.State != execution.Cancelled || calls.Load() != 0 {
				t.Fatal(snap, err, errors.Unwrap(err))
			}
			if scenario == "unavailable" && snap.Steps[0].Reason != "action_unavailable" {
				t.Fatal(snap)
			}
		})
	}
}

type timeoutLog struct {
	expired chan struct{}
	once    atomic.Bool
}

func (l *timeoutLog) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "shutdown timed out") && l.once.CompareAndSwap(false, true) {
		close(l.expired)
	}
	return len(p), nil
}

func TestDispatcherRetainsOwnershipUntilUncooperativeHandlerEnds(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	d, s := dispatchFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
		close(started)
		<-release
		return map[string]any{}, nil
	}, source)
	d.Capacity.ShutdownTimeout = time.Second
	log := &timeoutLog{expired: make(chan struct{})}
	d.Logger = slog.New(slog.NewJSONHandler(log, nil))
	q, err := d.Engine.Enqueue(ctx, s, d.Capacity, "example", RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := d.Run(runCtx, q.Snapshot.ID); done <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		close(release)
		t.Fatal(ctx.Err())
	}
	stop()
	select {
	case <-log.expired:
	case err := <-done:
		close(release)
		t.Fatal("returned while handler still running", err)
	case <-ctx.Done():
		close(release)
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-done:
		close(release)
		t.Fatal("owner escaped", err)
	default:
	}
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	next, err := s.BeginDispatcher(ctx, strings.Repeat("b", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverDispatcher(ctx, next); err != nil {
		t.Fatal(err)
	}
	snap, err := s.Execution(ctx, q.Snapshot.ID)
	if err != nil || snap.State != execution.Failed || !snap.UncertainEffects {
		t.Fatal(snap, err)
	}
}

func TestDispatcherInvalidAdmissionNeverInvokes(t *testing.T) {
	e := Engine{}
	if _, err := e.Enqueue(t.Context(), nil, execution.DefaultCapacity(), "example", RunOptions{}); !errors.Is(err, execution.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := (Dispatcher{}).Run(t.Context(), ""); !errors.Is(err, execution.ErrInvalid) {
		t.Fatal(err)
	}
	d, s := dispatchFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
		t.Error("unexpected invocation")
		return nil, nil
	}, source)
	d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, options := range []RunOptions{{Parent: "bad"}, {IdempotencyKey: "has spaces"}} {
		if _, err := d.Engine.Enqueue(t.Context(), s, d.Capacity, "example", options); err == nil {
			t.Fatal(options)
		}
	}
	if _, err := d.Engine.Enqueue(t.Context(), s, d.Capacity, "absent", RunOptions{}); !errors.Is(err, registry.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestDispatcherParallelDependenciesAndWorkerBound(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	started := make(chan string, 3)
	release := make(chan struct{})
	var active, peak atomic.Int32
	d, s := dispatchFixture(t, func(ctx context.Context, in map[string]any) (map[string]any, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- in["message"].(string)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return map[string]any{"value": in["message"]}, nil
	}, parallelSource)
	d.Capacity.Workers = 2
	d.Capacity.ActiveSteps = 2
	q, err := d.Engine.Enqueue(ctx, s, d.Capacity, "example", RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		snap execution.Snapshot
		err  error
	}
	done := make(chan result, 1)
	go func() { snap, err := d.Run(ctx, q.Snapshot.ID); done <- result{snap, err} }()
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(release)
	select {
	case r := <-done:
		if r.err != nil || r.snap.State != execution.Succeeded || string(r.snap.Steps[2].Output) != `{"value":"first"}` {
			t.Fatal(r.snap, r.err, errors.Unwrap(r.err))
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if peak.Load() != 2 || active.Load() != 0 {
		t.Fatal(peak.Load(), active.Load())
	}
}

func TestDispatcherBackoffReleasesWorkerAndStepCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	second := make(chan struct{})
	var first atomic.Int32
	doc := strings.Replace(parallelSource, "needs: [], with: {message: first}", "needs: [], retry: {attempts: 2, backoff: 1h, maxBackoff: 1h}, with: {message: first}", 1)
	d, s := dispatchFixture(t, func(ctx context.Context, in map[string]any) (map[string]any, error) {
		if in["message"] == "first" {
			first.Add(1)
			return nil, errors.New("private failure")
		}
		close(second)
		return map[string]any{"value": "second"}, nil
	}, doc)
	d.Capacity.Workers = 1
	q, err := d.Engine.Enqueue(ctx, s, d.Capacity, "example", RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := d.Run(ctx, q.Snapshot.ID); done <- err }()
	select {
	case <-second:
	case <-ctx.Done():
		t.Fatal("backoff held worker", ctx.Err())
	}
	if _, err = s.RequestReliableCancellation(ctx, q.Snapshot.ID, "first"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err, errors.Unwrap(err))
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if first.Load() != 1 {
		t.Fatal(first.Load())
	}
	snap, err := s.Execution(ctx, q.Snapshot.ID)
	if err != nil || snap.Steps[0].State != execution.Cancelled || snap.Steps[1].State != execution.Succeeded || snap.Steps[2].State != execution.Skipped {
		t.Fatal(snap, err)
	}
}

func TestDispatcherServerCapacityAndShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	started := make(chan struct{}, 4)
	var active atomic.Int32
	d, s := dispatchFixture(t, func(ctx context.Context, in map[string]any) (map[string]any, error) {
		active.Add(1)
		defer active.Add(-1)
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}, parallelSource)
	d.Capacity.Workers = 2
	d.Capacity.RunningExecutions = 2
	admissions := []execution.Snapshot{}
	for range 3 {
		q, err := d.Engine.Enqueue(ctx, s, d.Capacity, "example", RunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		admissions = append(admissions, q.Snapshot)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := d.Run(runCtx, ""); done <- err }()
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	// Claim limits are enforced in SQL regardless of how many action workers are busy.
	last, err := s.Execution(ctx, admissions[2].ID)
	if err != nil || last.State != execution.Queued {
		t.Fatal(last, err)
	}
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err, errors.Unwrap(err))
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if active.Load() != 0 {
		t.Fatal("worker escaped shutdown")
	}
	last, err = s.Execution(ctx, admissions[2].ID)
	if err != nil || last.State != execution.Queued {
		t.Fatal("shutdown claimed waiting work", last, err)
	}
}

type dispatchFault struct {
	execution.DispatchRepository
	store         *sqlite.Store
	operation     execution.Operation
	after, cancel bool
}

func (f *dispatchFault) ChangeDispatch(ctx context.Context, token execution.Fence, m execution.ReliableMutation) (execution.Snapshot, error) {
	if m.Operation != f.operation {
		return f.DispatchRepository.ChangeDispatch(ctx, token, m)
	}
	f.operation = ""
	if f.cancel {
		if _, err := f.store.RequestReliableCancellation(ctx, m.ID, ""); err != nil {
			return execution.Snapshot{}, err
		}
		return f.DispatchRepository.ChangeDispatch(ctx, token, m)
	}
	if f.after {
		snap, err := f.DispatchRepository.ChangeDispatch(ctx, token, m)
		if err != nil {
			return snap, err
		}
		return snap, errors.New("ambiguous commit")
	}
	return execution.Snapshot{}, errors.New("private SQL failure")
}

func TestDispatcherPersistenceBoundaries(t *testing.T) {
	for _, scenario := range []string{"before_attempt", "after_finish", "cancel_race"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			d, s := dispatchFixture(t, func(context.Context, map[string]any) (map[string]any, error) {
				calls.Add(1)
				return map[string]any{}, nil
			}, source)
			fault := &dispatchFault{DispatchRepository: s, store: s, operation: execution.StartAttempt}
			if scenario == "after_finish" {
				fault.operation = execution.FinishAttempt
				fault.after = true
			}
			if scenario == "cancel_race" {
				fault.cancel = true
			}
			d.Repository = fault
			q, err := d.Engine.Enqueue(t.Context(), s, d.Capacity, "example", RunOptions{})
			if err != nil {
				t.Fatal(err)
			}
			result, runErr := d.Run(t.Context(), q.Snapshot.ID)
			err = runErr
			if result.ID != q.Snapshot.ID || result.Workflow != q.Snapshot.Workflow {
				t.Fatal("lost admission receipt on failure", result, err)
			}
			if err == nil {
				t.Fatal("failure hidden")
			}
			want := int32(0)
			if fault.after {
				want = 1
			}
			if calls.Load() != want {
				t.Fatal(calls.Load(), want, err)
			}
			snap, readErr := s.Execution(t.Context(), q.Snapshot.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if scenario == "cancel_race" && snap.State != execution.Cancelled {
				t.Fatal(snap, err)
			}
			if scenario == "after_finish" && snap.Steps[0].State != execution.Succeeded {
				t.Fatal(snap, err)
			}
		})
	}
}

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

const source = "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - {id: first, uses: test.echo, with: {message: hello}}\n    - {id: second, uses: test.echo, with: {message: world}}\n"

func fixture(t *testing.T, h action.Handler) (Engine, *sqlite.Store, string) {
	t.Helper()
	d := registry.Descriptor{ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"message": {Type: "string", Required: true}}}, Outputs: map[string]registry.Field{"value": {Type: "string"}, "secret": {Type: "string", Sensitive: true}}}
	r, err := action.New([]action.Registration{{Descriptor: d, Run: h}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "db.sqlite")
	s, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	candidates, err := registry.Prepare(t.Context(), []registry.Source{{Data: []byte(source)}}, registry.Catalog{d.ID: d})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(t.Context(), candidates); err != nil {
		t.Fatal(err)
	}
	return Engine{Workflows: s, Executions: s, Actions: r}, s, path
}

func TestDurableManualExecutionHistory(t *testing.T) {
	calls := 0
	e, s, path := fixture(t, func(ctx context.Context, in map[string]any) (map[string]any, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Error("no deadline")
		}
		return map[string]any{"value": in["message"], "secret": "redact-me"}, nil
	})
	if _, err := s.SetEnabled(t.Context(), "example", false); err != nil {
		t.Fatal(err)
	}
	snap, err := e.Run(t.Context(), "example")
	if err != nil || calls != 2 || snap.State != execution.Succeeded {
		t.Fatal(snap, err, calls, errors.Unwrap(err))
	}
	if string(snap.Steps[0].Output) != `{"value":"hello"}` || snap.Steps[0].Attempt != 1 {
		t.Fatal(snap.Steps)
	}
	original := snap.Revision
	catalog, err := e.Actions.Catalog(map[string]string{"test.echo": "1"})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := registry.Prepare(t.Context(), []registry.Source{{Data: []byte(strings.ReplaceAll(source, "hello", "updated"))}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(t.Context(), candidates); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(t.Context(), "example"); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := reopened.Execution(t.Context(), snap.ID)
	if err != nil || got.Revision != original || !strings.Contains(string(got.Definition), "hello") {
		t.Fatal(got, err)
	}
	events, err := reopened.Events(t.Context(), snap.ID)
	if err != nil || len(events) != 8 {
		t.Fatal(events, err)
	}
	data, err := json.Marshal(events)
	if err != nil || strings.Contains(string(data), "redact-me") {
		t.Fatal(string(data), err)
	}
	list, err := reopened.Executions(t.Context())
	if err != nil || len(list) != 1 || list[0].Steps != nil || list[0].Definition != nil {
		t.Fatal(list, err)
	}
	if _, err := e.Run(t.Context(), "example"); !errors.Is(err, registry.ErrArchived) {
		t.Fatal(err)
	}
}

func TestActionFailureAndInvalidOutput(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "action", true: "output"}[invalid], func(t *testing.T) {
			calls := 0
			e, s, _ := fixture(t, func(context.Context, map[string]any) (map[string]any, error) {
				calls++
				if invalid {
					return map[string]any{"unknown": "secret"}, nil
				}
				return nil, errors.New("secret action detail")
			})
			snap, err := e.Run(t.Context(), "example")
			if !errors.Is(err, execution.ErrAction) || calls != 1 || snap.State != execution.Failed || snap.Steps[1].State != execution.Skipped {
				t.Fatal(snap, err, calls)
			}
			want := "action_failed"
			if invalid {
				want = "invalid_output"
			}
			if snap.Steps[0].Reason != want {
				t.Fatal(snap.Steps)
			}
			events, err := s.Events(t.Context(), snap.ID)
			b, _ := json.Marshal(events)
			if err != nil || strings.Contains(string(b), "secret") {
				t.Fatal(string(b), err)
			}
		})
	}
}

func TestCancellationAndDeadlinePersistence(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			e, s, _ := fixture(t, func(actionCtx context.Context, _ map[string]any) (map[string]any, error) {
				if !deadline {
					cancel()
				}
				<-actionCtx.Done()
				return nil, actionCtx.Err()
			})
			if deadline {
				ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
			} else {
				ctx, cancel = context.WithCancel(t.Context())
			}
			defer cancel()
			snap, err := e.Run(ctx, "example")
			want := execution.Cancelled
			cause := context.Canceled
			if deadline {
				want = execution.TimedOut
				cause = context.DeadlineExceeded
			}
			if !errors.Is(err, cause) || snap.State != want || snap.Steps[1].State != execution.Cancelled {
				t.Fatalf("execution %s has state %s at version %d, want %s: %v (persistence cause: %v)", snap.ID, snap.State, snap.Version, want, err, errors.Unwrap(err))
			}
			persisted, err := s.Execution(t.Context(), snap.ID)
			if err != nil || persisted.State != want {
				t.Fatal(persisted, err)
			}
		})
	}
}

func TestCancellationPersistenceHasIndependentBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var cancelledAt time.Time
	e, s, _ := fixture(t, func(actionCtx context.Context, _ map[string]any) (map[string]any, error) {
		cancelledAt = time.Now()
		cancel()
		return nil, actionCtx.Err()
	})
	checked := false
	e.Executions = changeObserver{Repository: s, observe: func(writeCtx context.Context, m execution.Mutation) {
		if m.Step != "first" || m.To != execution.Cancelled {
			return
		}
		checked = true
		deadline, ok := writeCtx.Deadline()
		if writeCtx.Err() != nil || !ok || deadline.Before(cancelledAt.Add(30*time.Second)) || deadline.After(time.Now().Add(30*time.Second)) {
			t.Errorf("expected a live, 30-second persistence budget after cancellation at %s, got deadline %s and error %v", cancelledAt, deadline, writeCtx.Err())
		}
	}}
	snap, err := e.Run(ctx, "example")
	if !checked || snap.State != execution.Cancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup observed=%t, state=%s, error=%v", checked, snap.State, err)
	}
}

type changeObserver struct {
	execution.Repository
	observe func(context.Context, execution.Mutation)
}

func (r changeObserver) Change(ctx context.Context, m execution.Mutation) (execution.Snapshot, error) {
	r.observe(ctx, m)
	return r.Repository.Change(ctx, m)
}

type failingRepository struct {
	execution.Repository
	failAt, calls int
	cause         error
}

func (r *failingRepository) Change(ctx context.Context, m execution.Mutation) (execution.Snapshot, error) {
	r.calls++
	if r.calls == r.failAt {
		return execution.Snapshot{}, r.cause
	}
	return r.Repository.Change(ctx, m)
}

func TestPersistenceFailuresNeverRepeatActions(t *testing.T) {
	for _, at := range []int{1, 2, 3, 5} {
		t.Run(string(rune('0'+at)), func(t *testing.T) {
			calls := 0
			e, s, _ := fixture(t, func(context.Context, map[string]any) (map[string]any, error) { calls++; return map[string]any{}, nil })
			cause := errors.New("private-storage-error")
			e.Executions = &failingRepository{Repository: s, failAt: at, cause: cause}
			snap, err := e.Run(t.Context(), "example")
			var failure *execution.Failure
			if !errors.As(err, &failure) || !errors.Is(err, cause) || strings.Contains(err.Error(), cause.Error()) {
				t.Fatal(err)
			}
			wantCalls := 0
			if at > 1 {
				wantCalls = 1
			}
			if at == 5 {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatal(calls)
			}
			list, err := s.Executions(t.Context())
			if err != nil || len(list) != 1 || list[0].State != execution.Running || snap.ID == "" {
				t.Fatal(list, err)
			}
			if _, err := e.Run(t.Context(), "example"); err != execution.ErrConflict {
				t.Fatal(err)
			}
		})
	}
}

func TestPreflightAndMissingConfiguration(t *testing.T) {
	if _, err := (Engine{}).Run(t.Context(), "example"); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (Engine{}).Run(ctx, "example"); err != context.Canceled {
		t.Fatal(err)
	}
	e, s, _ := fixture(t, func(context.Context, map[string]any) (map[string]any, error) {
		t.Fatal("invoked unavailable action")
		return nil, nil
	})
	e.Actions = nil
	if _, err := e.Run(t.Context(), "example"); err != execution.ErrUnavailable {
		t.Fatal(err)
	}
	if list, err := s.Executions(t.Context()); err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	if _, err := e.Run(t.Context(), "missing"); err != registry.ErrNotFound {
		t.Fatal(err)
	}
}

type alteredWorkflows struct {
	registry.Repository
	edit func(*registry.Record)
}

func (r alteredWorkflows) Get(ctx context.Context, name, revision string) (registry.Record, error) {
	record, err := r.Repository.Get(ctx, name, revision)
	if err == nil {
		r.edit(&record)
	}
	return record, err
}

func TestPreflightCorruptionRejectedBeforeSubmission(t *testing.T) {
	for _, edit := range []func(*registry.Record){func(r *registry.Record) { r.Bindings = json.RawMessage(`invalid`) }, func(r *registry.Record) { r.Bindings = json.RawMessage(`{"test.echo":"2"}`) }, func(r *registry.Record) { r.Source = []byte("invalid") }, func(r *registry.Record) { r.Definition = json.RawMessage(`{}`) }, func(r *registry.Record) { r.Name = "different" }} {
		e, s, _ := fixture(t, func(context.Context, map[string]any) (map[string]any, error) {
			t.Fatal("invalid preflight invoked action")
			return nil, nil
		})
		e.Workflows = alteredWorkflows{Repository: s, edit: edit}
		if _, err := e.Run(t.Context(), "example"); err == nil {
			t.Fatal("corrupt preflight accepted")
		}
		rows, err := s.Executions(t.Context())
		if err != nil || len(rows) != 0 {
			t.Fatal(rows, err)
		}
	}
}

type ambiguousRepository struct {
	execution.Repository
	submit bool
}

func (r ambiguousRepository) Submit(ctx context.Context, record registry.Record, owner string) (execution.Snapshot, error) {
	snap, err := r.Repository.Submit(ctx, record, owner)
	if err == nil && r.submit {
		return snap, errors.New("ambiguous commit")
	}
	return snap, err
}
func (r ambiguousRepository) Change(ctx context.Context, m execution.Mutation) (execution.Snapshot, error) {
	snap, err := r.Repository.Change(ctx, m)
	if err == nil {
		return snap, errors.New("ambiguous commit")
	}
	return snap, err
}

func TestAmbiguousCommitNeverInvokesOrResubmits(t *testing.T) {
	for _, submit := range []bool{false, true} {
		e, s, _ := fixture(t, func(context.Context, map[string]any) (map[string]any, error) {
			t.Fatal("invoked after ambiguous commit")
			return nil, nil
		})
		e.Executions = ambiguousRepository{Repository: s, submit: submit}
		snap, err := e.Run(t.Context(), "example")
		var failure *execution.Failure
		if snap.ID == "" || !errors.As(err, &failure) {
			t.Fatal(snap, err)
		}
		got, err := s.Execution(t.Context(), snap.ID)
		if err != nil || got.State != execution.Running {
			t.Fatal(got, err)
		}
		if _, err := e.Run(t.Context(), "example"); err != execution.ErrConflict {
			t.Fatal(err)
		}
	}
}

func TestCancellationCleanupFailureIsReported(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	e, s, _ := fixture(t, func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	})
	e.Executions = &failingRepository{Repository: s, failAt: 2, cause: context.DeadlineExceeded}
	snap, err := e.Run(ctx, "example")
	var failure *execution.Failure
	if !errors.As(err, &failure) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	got, err := s.Execution(t.Context(), snap.ID)
	if err != nil || got.State != execution.Running || got.Steps[0].State != execution.Running {
		t.Fatal(got, err)
	}
}

func TestStepOutcomePrecedence(t *testing.T) {
	cases := []struct {
		parent, step, action error
		oversized            bool
		state                execution.State
		reason               string
	}{
		{nil, nil, nil, false, execution.Succeeded, ""},
		{context.Canceled, context.DeadlineExceeded, execution.ErrOutput, false, execution.Cancelled, "cancelled"},
		{context.DeadlineExceeded, context.DeadlineExceeded, execution.ErrAction, false, execution.TimedOut, "deadline"},
		{nil, context.DeadlineExceeded, context.DeadlineExceeded, false, execution.TimedOut, "deadline"},
		{nil, nil, execution.ErrPermission, false, execution.Failed, "permission_denied"},
		{nil, nil, execution.ErrInvalid, false, execution.Failed, "invalid_output"},
		{nil, nil, execution.ErrAction, false, execution.Failed, "action_failed"},
		{nil, nil, nil, true, execution.Failed, "invalid_output"},
	}
	for _, tc := range cases {
		state, reason := stepOutcome(tc.parent, tc.step, tc.action, tc.oversized)
		if state != tc.state || reason != tc.reason {
			t.Fatal(state, reason, tc)
		}
	}
}

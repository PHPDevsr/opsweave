package execution

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

func reliableFixture() Snapshot {
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	deadline := now.Add(time.Hour)
	return Snapshot{ID: "0123456789abcdef0123456789abcdef", Format: 2, State: Running, CreatedAt: now, UpdatedAt: now, Deadline: &deadline, Steps: []Step{{ID: "one", State: Pending, Policy: &StepPolicy{Timeout: time.Minute, Retry: workflow.Retry{Attempts: 3, Backoff: time.Second, MaxBackoff: time.Minute}}}}}
}

func mutate(t *testing.T, s *Snapshot, op Operation, step string, now time.Time, outcome State, reason string, output json.RawMessage) []Event {
	t.Helper()
	events, err := ApplyReliable(s, ReliableMutation{Version: s.Version, Operation: op, Step: step, Outcome: outcome, Reason: reason, Output: output}, now)
	if err != nil {
		t.Fatal(op, err)
	}
	return events
}

func TestReliableAttemptsAndBackoff(t *testing.T) {
	s := reliableFixture()
	now := s.CreatedAt
	mutate(t, &s, StartStep, "one", now, "", "", nil)
	mutate(t, &s, StartAttempt, "one", now, "", "", nil)
	events := mutate(t, &s, FinishAttempt, "one", now, Failed, "action_failed", nil)
	if len(events) != 2 || events[1].Code != "retry_scheduled" || s.Steps[0].State != Running || s.Steps[0].Attempts[0].State != Failed || !s.Steps[0].NextAttemptAt.Equal(now.Add(time.Second)) {
		t.Fatal(s, events)
	}
	before, _ := json.Marshal(s)
	if _, err := ApplyReliable(&s, ReliableMutation{Version: s.Version, Operation: StartAttempt, Step: "one"}, now); err == nil {
		t.Fatal("early retry")
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("failed operation mutated snapshot")
	}
	now = now.Add(time.Second)
	mutate(t, &s, StartAttempt, "one", now, "", "", nil)
	mutate(t, &s, FinishAttempt, "one", now, Succeeded, "", json.RawMessage(`{"count":3}`))
	if s.Steps[0].Attempt != 2 || s.Steps[0].Attempts[1].State != Succeeded || s.Steps[0].Attempts[0].State != Failed {
		t.Fatal(s.Steps)
	}
	mutate(t, &s, FinishExecution, "", now, "", "", nil)
	if s.State != Succeeded {
		t.Fatal(s.State)
	}
	if _, err := ApplyReliable(&s, ReliableMutation{Version: s.Version, Operation: FinishExecution}, now); err != ErrConflict {
		t.Fatal(err)
	}
	if err := Advance(&s, Mutation{Version: s.Version, To: Failed}, now); err != ErrInvalid {
		t.Fatal("legacy state mutation accepted format 2", err)
	}
}

func TestReliableExhaustionAndNonRetryableResults(t *testing.T) {
	for _, reason := range []string{"action_failed", "invalid_output", "permission_denied"} {
		s := reliableFixture()
		now := s.CreatedAt
		s.Steps[0].Policy.Retry.Attempts = 1
		mutate(t, &s, StartStep, "one", now, "", "", nil)
		mutate(t, &s, StartAttempt, "one", now, "", "", nil)
		mutate(t, &s, FinishAttempt, "one", now, Failed, reason, nil)
		if s.Steps[0].State != Failed || s.Steps[0].NextAttemptAt != nil {
			t.Fatal(s.Steps)
		}
		mutate(t, &s, FinishExecution, "", now, "", "", nil)
		if s.State != Failed {
			t.Fatal(s)
		}
	}
}

func TestReliableConditionsDependenciesAndAggregate(t *testing.T) {
	s := reliableFixture()
	now := s.CreatedAt
	s.Steps[0].Policy.Condition = "false"
	mutate(t, &s, StartStep, "one", now, "", "", nil)
	if s.Steps[0].State != Skipped || s.Steps[0].Reason != "condition_false" || s.Steps[0].Attempt != 0 {
		t.Fatal(s.Steps)
	}
	mutate(t, &s, FinishExecution, "", now, "", "", nil)
	if s.State != Succeeded {
		t.Fatal(s.State)
	}
	s = reliableFixture()
	s.Steps[0].Policy.Condition = "steps.absent.success"
	mutate(t, &s, StartStep, "one", now, "", "", nil)
	if s.Steps[0].State != Failed || s.Steps[0].Reason != "condition_invalid" {
		t.Fatal(s.Steps)
	}
	s = reliableFixture()
	first := s.Steps[0]
	first.State = Failed
	first.Reason = "action_failed"
	second := reliableFixture().Steps[0]
	second.ID = "handler"
	second.Policy.Needs = []string{"one"}
	second.Policy.Condition = "steps.one.failed"
	s.Steps = []Step{first, second}
	mutate(t, &s, StartStep, "handler", now, "", "", nil)
	mutate(t, &s, StartAttempt, "handler", now, "", "", nil)
	mutate(t, &s, FinishAttempt, "handler", now, Succeeded, "", json.RawMessage(`{}`))
	mutate(t, &s, FinishExecution, "", now, "", "", nil)
	if s.State != Failed {
		t.Fatal("handler erased failure")
	}
	s = reliableFixture()
	first = s.Steps[0]
	first.State = Failed
	second = reliableFixture().Steps[0]
	second.ID = "dependent"
	second.Policy.Needs = []string{"one"}
	s.Steps = []Step{first, second}
	mutate(t, &s, StartStep, "dependent", now, "", "", nil)
	if s.Steps[1].State != Skipped || s.Steps[1].Reason != "dependency_failed" {
		t.Fatal(s.Steps)
	}
}

func TestReliableCancellationDeadlineAndResolution(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		s := reliableFixture()
		now := s.CreatedAt
		mutate(t, &s, StartStep, "one", now, "", "", nil)
		mutate(t, &s, StartAttempt, "one", now, "", "", nil)
		if cancel {
			s.Steps[0].CancelRequestedAt = stamp(now)
		} else {
			now = now.Add(2 * time.Minute)
		}
		mutate(t, &s, FinishAttempt, "one", now, Succeeded, "", json.RawMessage(`{"private":"discard"}`))
		want := TimedOut
		if cancel {
			want = Cancelled
		}
		if s.Steps[0].State != want || len(s.Steps[0].Output) != 0 {
			t.Fatal(s.Steps)
		}
		mutate(t, &s, FinishExecution, "", now, "", "", nil)
		overall := Failed
		if cancel {
			overall = Cancelled
		}
		if s.State != overall {
			t.Fatal(s.State)
		}
	}
	s := reliableFixture()
	now := s.CreatedAt
	s.CancelRequestedAt = stamp(now)
	mutate(t, &s, CancelIdleStep, "one", now, "", "", nil)
	mutate(t, &s, FinishExecution, "", now, "", "", nil)
	if s.State != Cancelled {
		t.Fatal(s.State)
	}
	s = reliableFixture()
	mutate(t, &s, StartStep, "one", now, "", "", nil)
	mutate(t, &s, FailResolution, "one", now, "", "reference_unavailable", nil)
	if s.Steps[0].State != Failed || s.Steps[0].Attempt != 0 {
		t.Fatal(s.Steps)
	}
	s = reliableFixture()
	mutate(t, &s, StartStep, "one", now, "", "", nil)
	mutate(t, &s, StartAttempt, "one", now, "", "", nil)
	mutate(t, &s, FinishAttempt, "one", now, Failed, "action_failed", nil)
	mutate(t, &s, StartAttempt, "one", now.Add(time.Hour), "", "", nil)
	if s.Steps[0].State != TimedOut || s.Steps[0].Attempt != 1 {
		t.Fatal(s.Steps)
	}
}

func TestReliableCancellationTimingAndRunningCapacity(t *testing.T) {
	for _, early := range []bool{false, true} {
		s := reliableFixture()
		at := s.CreatedAt
		mutate(t, &s, StartStep, "one", at, "", "", nil)
		mutate(t, &s, StartAttempt, "one", at, "", "", nil)
		request := at.Add(2 * time.Hour)
		if early {
			request = at
		}
		s.CancelRequestedAt = &request
		mutate(t, &s, FinishAttempt, "one", at.Add(3*time.Hour), Succeeded, "", json.RawMessage(`{}`))
		mutate(t, &s, FinishExecution, "", at.Add(3*time.Hour), "", "", nil)
		want := TimedOut
		if early {
			want = Cancelled
		}
		if s.State != want {
			t.Fatal(s.State, early)
		}
	}
	s := reliableFixture()
	other := reliableFixture().Steps[0]
	other.ID = "two"
	s.Steps = append(s.Steps, other)
	mutate(t, &s, StartStep, "one", s.CreatedAt, "", "", nil)
	if _, err := ApplyReliable(&s, ReliableMutation{Version: s.Version, Step: "two", Operation: StartStep}, s.CreatedAt); err != ErrInvalid {
		t.Fatal("parallelism escaped pre-dispatcher gate", err)
	}
}

func TestReliableInvalidOperationsAreAtomic(t *testing.T) {
	for _, tc := range []struct {
		edit func(*Snapshot)
		m    ReliableMutation
	}{
		{nil, ReliableMutation{Step: "unknown", Operation: StartStep}},
		{nil, ReliableMutation{Step: "one", Operation: "unknown"}},
		{nil, ReliableMutation{Step: "one", Operation: StartAttempt}},
		{nil, ReliableMutation{Step: "one", Operation: FinishAttempt, Outcome: Succeeded, Output: json.RawMessage(`{}`)}},
		{nil, ReliableMutation{Step: "one", Operation: StartStep, Output: json.RawMessage(`{}`)}},
		{nil, ReliableMutation{Operation: FinishExecution}},
		{nil, ReliableMutation{Step: "one", Operation: CancelIdleStep}},
		{func(s *Snapshot) { s.Format = 1 }, ReliableMutation{Step: "one", Operation: StartStep}},
		{func(s *Snapshot) { s.Deadline = nil }, ReliableMutation{Step: "one", Operation: StartStep}},
		{func(s *Snapshot) { s.Steps[0].Policy = nil }, ReliableMutation{Step: "one", Operation: StartStep}},
		{func(s *Snapshot) { s.Steps[0].Policy.Needs = []string{"unknown"} }, ReliableMutation{Step: "one", Operation: StartStep}},
		{func(s *Snapshot) { s.Steps[0].State = Succeeded }, ReliableMutation{Step: "one", Operation: StartStep}},
		{func(s *Snapshot) { s.Version = 2 }, ReliableMutation{Step: "one", Operation: StartStep}},
	} {
		s := reliableFixture()
		if tc.edit != nil {
			tc.edit(&s)
		}
		before, _ := json.Marshal(s)
		_, err := ApplyReliable(&s, tc.m, s.CreatedAt)
		after, _ := json.Marshal(s)
		if err == nil || !bytesEqual(before, after) {
			t.Fatal(tc.m, err)
		}
	}
}
func bytesEqual(a, b []byte) bool { return string(a) == string(b) }

func TestRetryDelayBounds(t *testing.T) {
	policy := workflow.Retry{Attempts: 10, Backoff: time.Second, MaxBackoff: 5 * time.Second}
	for finished, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 5 * time.Second, 9: 5 * time.Second} {
		got, err := RetryDelay(policy, finished)
		if err != nil || got != want {
			t.Fatal(got, err)
		}
	}
	overflow := workflow.Retry{Attempts: 10, Backoff: time.Duration(math.MaxInt64/2 + 1), MaxBackoff: time.Duration(math.MaxInt64)}
	if got, err := RetryDelay(overflow, 2); err != nil || got != overflow.MaxBackoff {
		t.Fatal(got, err)
	}
	for _, finished := range []int{0, 10, -1} {
		if _, err := RetryDelay(policy, finished); err == nil {
			t.Fatal(finished)
		}
	}
}

func TestAdmissionAndRecoveryClassification(t *testing.T) {
	record := registry.Record{Revision: "0123456789abcdef0123456789abcdef", Bindings: json.RawMessage(`{"test.echo":"1"}`)}
	a, err := AdmissionFor("private-key", record, "", false)
	b, err2 := AdmissionFor("private-key", record, "", false)
	if err != nil || err2 != nil || a != b || len(a.Digest) != 64 || strings.Contains(a.Digest, "private-key") {
		t.Fatal(a, err)
	}
	for _, key := range []string{"has space", "\n", strings.Repeat("x", 129), "é"} {
		if _, err := AdmissionFor(key, record, "", false); err != ErrInvalid {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		edit func(*Snapshot)
		want string
	}{
		{nil, "resumable"}, {func(s *Snapshot) { s.State = Succeeded }, "terminal"},
		{func(s *Snapshot) { s.Format = 9 }, "invalid"},
		{func(s *Snapshot) { s.Steps[0].State = Running; s.Format = 0 }, "uncertain_effect"},
		{func(s *Snapshot) { s.Steps[0].State = Running; s.Steps[0].Attempts = []Attempt{{State: Running}} }, "uncertain_effect"},
		{func(s *Snapshot) { s.Deadline = stamp(s.CreatedAt.Add(-time.Second)) }, "expired"},
		{func(s *Snapshot) { s.Steps[0].State = Running; s.Steps[0].Attempts = []Attempt{{State: Succeeded}} }, "invalid"},
	} {
		s := reliableFixture()
		if tc.edit != nil {
			tc.edit(&s)
		}
		if got := ClassifyRecovery(s, s.CreatedAt); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
}

func TestNewReliablePinsAndGates(t *testing.T) {
	source := []byte("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - {id: one, uses: test.echo, retry: {attempts: 2}}\n")
	d, err := workflow.Parse(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := registry.Normalize(d)
	if err != nil {
		t.Fatal(err)
	}
	record := registry.Record{Name: "example", Revision: "0123456789abcdef0123456789abcdef", Source: source, Definition: normalized, Bindings: json.RawMessage(`{"test.echo":"1"}`)}
	now := time.Now().UTC()
	ctx, cancel := context.WithDeadline(t.Context(), now.Add(time.Minute))
	defer cancel()
	snap, err := NewReliable(ctx, record.Revision, record, now)
	if err != nil || snap.Format != 2 || !snap.Deadline.Equal(now.Add(time.Minute)) || snap.Steps[0].Policy.Retry.Attempts != 2 {
		t.Fatal(snap, err)
	}
	if _, err := NewReliable(t.Context(), "bad", record, now); err != ErrInvalid {
		t.Fatal(err)
	}
	record.Definition = json.RawMessage(`{}`)
	if _, err := NewReliable(t.Context(), record.Revision, record, now); err != ErrInvalid {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snap.Steps[0].Policy.Needs, []string{}) {
		t.Fatal(snap.Steps)
	}
	if _, err := NewReliable(ctx, snap.ID, record, now.Add(time.Hour)); err == nil || !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

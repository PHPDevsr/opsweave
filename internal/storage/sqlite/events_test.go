package sqlite

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/event"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

func TestInternalEventBatchRollbackAndReplay(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	source := strings.Replace(string(r.Source), "spec:", "spec:\n  triggers: [{id: first, type: internal, event: build}, {id: second, type: internal, event: build}]", 1)
	r = dispatcherRevision(t, s, []byte(source))
	cap := execution.DefaultCapacity()
	cap.QueuedExecutions = 1
	input := event.Request{Type: "internal", Name: "build", ID: "unique-private-identifier"}
	rows, err := s.EmitEvent(t.Context(), input, cap, nil)
	if !errors.Is(err, execution.ErrQueueFull) || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	jobs, err := s.Executions(t.Context())
	if err != nil || len(jobs) != 0 {
		t.Fatal(jobs, err)
	}
	history, err := s.EventHistory(t.Context(), r.Name, 100)
	if err != nil || len(history) != 1 || history[0].Code != "queue_full" {
		t.Fatal(history, err)
	}
	cap.QueuedExecutions = 2
	rows, err = s.EmitEvent(t.Context(), input, cap, nil)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	updated := dispatcherRevision(t, s, []byte(strings.Replace(source, "message: hello", "message: changed", 1)))
	replay, err := s.EmitEvent(t.Context(), input, cap, nil)
	if err != nil || len(replay) != 2 || replay[0] != rows[0] || replay[1] != rows[1] {
		t.Fatal(replay, err)
	}
	job, err := s.Execution(t.Context(), rows[0].Execution)
	if err != nil || job.Trigger == nil || job.Trigger.Type != "internal" || job.Revision == updated.Revision {
		t.Fatal(job, err)
	}
	var digest string
	if err = s.db.QueryRowContext(t.Context(), "SELECT event_digest FROM event_dedup LIMIT 1").Scan(&digest); err != nil || digest != event.Digest([]byte(input.ID)) {
		t.Fatal(digest, err)
	}
}

func TestWebhookReplayConflictExpiryAndLifecycle(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	source := strings.Replace(string(r.Source), "spec:", "spec:\n  triggers: [{id: hook, type: webhook}]", 1)
	r = dispatcherRevision(t, s, []byte(source))
	fence, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	cap := execution.DefaultCapacity()
	input := event.Request{Type: "webhook", Workflow: r.Name, Trigger: "hook", ID: "one", Fingerprint: event.Digest([]byte(`{}`))}
	first, err := s.EmitEvent(t.Context(), input, cap, &fence)
	if err != nil || len(first) != 1 {
		t.Fatal(first, err)
	}
	input.Fingerprint = event.Digest([]byte(`{"changed":true}`))
	if _, err = s.EmitEvent(t.Context(), input, cap, &fence); !errors.Is(err, event.ErrConflict) {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(t.Context(), "UPDATE event_dedup SET expires_at=0"); err != nil {
		t.Fatal(err)
	}
	second, err := s.EmitEvent(t.Context(), input, cap, &fence)
	if err != nil || len(second) != 1 || first[0] == second[0] {
		t.Fatal(second, err)
	}
	stale := fence
	stale.Generation++
	if _, err = s.EmitEvent(t.Context(), input, cap, &stale); !errors.Is(err, execution.ErrConflict) {
		t.Fatal(err)
	}
	if _, err = s.SetEnabled(t.Context(), r.Name, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.EmitEvent(t.Context(), input, cap, &fence); !errors.Is(err, event.ErrRoute) {
		t.Fatal(err)
	}
	if _, err = s.SetEnabled(t.Context(), r.Name, true); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimExecution(t.Context(), fence, second[0].Execution, r, cap)
	if err != nil || job.State != execution.Cancelled {
		t.Fatal(job, err)
	}
}

func TestStartupScanResumesAfterQueueFull(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	r = dispatcherRevision(t, s, []byte(strings.Replace(string(r.Source), "spec:", "spec:\n  triggers: [{id: ready, type: startup}]", 1)))
	fence, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	cap := execution.DefaultCapacity()
	cap.QueuedExecutions = 1
	manual, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BeginStartup(t.Context(), fence); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if done, err := s.AdvanceStartup(t.Context(), fence, cap); err != nil || done {
			t.Fatal(done, err)
		}
	}
	history, err := s.EventHistory(t.Context(), r.Name, 100)
	if err != nil || len(history) != 1 || history[0].Code != "queue_full" {
		t.Fatal(history, err)
	}
	if _, err = s.RequestReliableCancellation(t.Context(), manual.Snapshot.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimExecution(t.Context(), fence, manual.Snapshot.ID, r, cap); err != nil {
		t.Fatal(err)
	}
	if done, err := s.AdvanceStartup(t.Context(), fence, cap); err != nil || !done {
		t.Fatal(done, err)
	}
	if err = s.BeginStartup(t.Context(), fence); err != nil {
		t.Fatal(err)
	}
	if done, err := s.AdvanceStartup(t.Context(), fence, cap); err != nil || !done {
		t.Fatal(done, err)
	}
	history, err = s.EventHistory(t.Context(), r.Name, 100)
	if err != nil || len(history) != 2 || history[0].Type != "startup" || history[0].Execution == "" {
		t.Fatal(history, err)
	}
}

func TestInternalEventDestinationBound(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	for i := range 5 {
		source := strings.Replace(string(r.Source), "name: "+r.Name, fmt.Sprintf("name: event-%d", i), 1)
		triggers := "spec:\n  triggers:\n"
		for n := range 16 {
			triggers += fmt.Sprintf("    - {id: each-%d, type: internal, event: build}\n", n)
		}
		dispatcherRevision(t, s, []byte(strings.Replace(source, "spec:", strings.TrimSuffix(triggers, "\n"), 1)))
	}
	if _, err := s.EmitEvent(t.Context(), event.Request{Type: "internal", Name: "build", ID: "one"}, execution.DefaultCapacity(), nil); !errors.Is(err, event.ErrDestinations) {
		t.Fatal(err)
	}
	jobs, err := s.Executions(t.Context())
	if err != nil || len(jobs) != 0 {
		t.Fatal(jobs, err)
	}
}

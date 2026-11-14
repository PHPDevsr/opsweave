package sqlite

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

func TestDispatcherAdmissionClaimAndFencing(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	cap := execution.DefaultCapacity()
	cap.QueuedExecutions = 1
	q := execution.QueueAdmission{Capacity: cap, Options: execution.AdmissionOptions{IdempotencyKey: "private-admission-key"}}
	first, err := s.Enqueue(t.Context(), r, q)
	if err != nil || !first.Created || first.Snapshot.State != execution.Queued || first.Snapshot.Deadline != nil {
		t.Fatal(first, err)
	}
	duplicate, err := s.Enqueue(t.Context(), r, q)
	if err != nil || duplicate.Created || duplicate.Snapshot.ID != first.Snapshot.ID {
		t.Fatal(duplicate, err)
	}
	q.Options.IdempotencyKey = "different"
	if _, err = s.Enqueue(t.Context(), r, q); !errors.Is(err, execution.ErrQueueFull) {
		t.Fatal(err)
	}
	candidates, err := s.QueuedExecutions(t.Context(), "")
	if err != nil || len(candidates) != 1 {
		t.Fatal(candidates, err)
	}
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.ClaimExecution(t.Context(), f, first.Snapshot.ID, r, cap)
	if err != nil || snap.State != execution.Running || snap.Deadline == nil {
		t.Fatal(snap, err)
	}
	m := execution.ReliableMutation{ID: snap.ID, Owner: f.Owner, Version: snap.Version, Step: "first", Operation: execution.StartStep}
	if _, err = s.ChangeReliable(t.Context(), m); !errors.Is(err, execution.ErrConflict) {
		t.Fatal("unfenced managed mutation", err)
	}
	started, err := s.ChangeDispatch(t.Context(), f, m)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := s.BeginDispatcher(t.Context(), strings.Repeat("b", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	m.Version = started.Version
	if _, err = s.ChangeDispatch(t.Context(), f, m); !errors.Is(err, execution.ErrConflict) {
		t.Fatal("stale generation", err)
	}
	if err = s.EndDispatcher(t.Context(), f); !errors.Is(err, execution.ErrConflict) {
		t.Fatal("stale release", err)
	}
	if err = s.RecoverDispatcher(t.Context(), newer); err != nil {
		t.Fatal(err)
	}
	got, err := s.DispatchExecutions(t.Context(), newer)
	if err != nil || len(got) != 1 || got[0].Steps[0].State != execution.Running {
		t.Fatal(got, err)
	}
	if err = s.EndDispatcher(t.Context(), newer); err != nil {
		t.Fatal(err)
	}
}

func dispatcherRevision(t *testing.T, s *Store, source []byte) registry.Record {
	t.Helper()
	d, err := workflow.Parse(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := registry.Normalize(d)
	if err != nil {
		t.Fatal(err)
	}
	records, err := s.Apply(t.Context(), []registry.Candidate{{Name: d.Metadata.Name, Source: source, Normalized: normalized, Bindings: map[string]string{"test.echo": "1"}, Capabilities: []string{"dispatcher"}}})
	if err != nil {
		t.Fatal(err)
	}
	return records[0]
}

func TestDispatcherRevisionAndGlobalCapacity(t *testing.T) {
	s, r, path := reliableStoreFixture(t)
	other := openTest(t, path)
	cap := execution.DefaultCapacity()
	cap.RunningExecutions = 2
	original := r.Source
	r = dispatcherRevision(t, s, []byte(strings.Replace(string(original), "spec:", "spec:\n  concurrency: {limit: 2}", 1)))
	q1, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	q2, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.ClaimExecution(t.Context(), f, q1.Snapshot.ID, r, cap)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	claims := make(chan error, 2)
	for _, store := range []*Store{s, other} {
		wg.Go(func() { _, err := store.ClaimExecution(t.Context(), f, q2.Snapshot.ID, r, cap); claims <- err })
	}
	wg.Wait()
	a, b := <-claims, <-claims
	if (a == nil) == (b == nil) {
		t.Fatal("claim winner count", a, b)
	}
	restrictive := dispatcherRevision(t, s, original)
	q3, err := s.Enqueue(t.Context(), restrictive, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimExecution(t.Context(), f, q3.Snapshot.ID, restrictive, cap); !errors.Is(err, execution.ErrBusy) {
		t.Fatal("global capacity", err)
	}
	for _, id := range []string{first.ID, q2.Snapshot.ID} {
		snap, err := s.RequestReliableCancellation(t.Context(), id, "")
		if err != nil {
			t.Fatal(err)
		}
		snap, err = s.ChangeDispatch(t.Context(), f, execution.ReliableMutation{ID: id, Owner: f.Owner, Version: snap.Version, Operation: execution.CancelIdleSteps})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.ChangeDispatch(t.Context(), f, execution.ReliableMutation{ID: id, Owner: f.Owner, Version: snap.Version, Operation: execution.FinishExecution}); err != nil {
			t.Fatal(err)
		}
		if id == first.ID {
			if _, err = s.ClaimExecution(t.Context(), f, q3.Snapshot.ID, restrictive, cap); !errors.Is(err, execution.ErrBusy) {
				t.Fatal("new restrictive revision ignored", err)
			}
		}
	}
	if _, err = s.ClaimExecution(t.Context(), f, q3.Snapshot.ID, restrictive, cap); err != nil {
		t.Fatal(err)
	}
	// A permissive historical candidate must respect the already-running restrictive revision.
	q4, err := s.Enqueue(t.Context(), restrictive, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	cap.RunningExecutions = 3
	if _, err = s.ClaimExecution(t.Context(), f, q4.Snapshot.ID, restrictive, cap); !errors.Is(err, execution.ErrBusy) {
		t.Fatal(err)
	}
}

func TestDispatcherManagedBackoffRecoveryPreservesBudget(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	cap := execution.DefaultCapacity()
	q, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.ClaimExecution(t.Context(), f, q.Snapshot.ID, r, cap)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []execution.ReliableMutation{{Step: "first", Operation: execution.StartStep}, {Step: "first", Operation: execution.StartAttempt}, {Step: "first", Operation: execution.FinishAttempt, Outcome: execution.Failed, Reason: "action_failed"}} {
		m.ID, m.Owner, m.Version = snap.ID, f.Owner, snap.Version
		snap, err = s.ChangeDispatch(t.Context(), f, m)
		if err != nil {
			t.Fatal(err)
		}
	}
	next, err := s.BeginDispatcher(t.Context(), strings.Repeat("c", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverDispatcher(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	after, err := s.Execution(t.Context(), snap.ID)
	if err != nil || !after.Deadline.Equal(*snap.Deadline) || !after.Steps[0].Deadline.Equal(*snap.Steps[0].Deadline) || !after.Steps[0].NextAttemptAt.Equal(*snap.Steps[0].NextAttemptAt) || len(after.Steps[0].Attempts) != 1 || after.Steps[0].Attempts[0].State != execution.Failed {
		t.Fatal(after, err)
	}
	if _, err = s.ChangeDispatch(t.Context(), f, execution.ReliableMutation{ID: snap.ID, Owner: f.Owner, Version: after.Version, Step: "first", Operation: execution.StartAttempt}); !errors.Is(err, execution.ErrConflict) {
		t.Fatal(err)
	}
}

func TestDispatcherConcurrentAdmissionAndExpiry(t *testing.T) {
	s, r, path := reliableStoreFixture(t)
	other := openTest(t, path)
	cap := execution.DefaultCapacity()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	ids := make(chan string, 2)
	created := make(chan bool, 2)
	for _, store := range []*Store{s, other} {
		wg.Go(func() {
			q, e := store.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap, Options: execution.AdmissionOptions{IdempotencyKey: "same"}})
			errs <- e
			ids <- q.Snapshot.ID
			created <- q.Created
		})
	}
	wg.Wait()
	a, b := <-ids, <-ids
	if a != b || a == "" {
		t.Fatal(a, b)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if (<-created) == (<-created) {
		t.Fatal("not exactly one admission winner")
	}
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(t.Context(), "UPDATE execution_queue SET expires_at=?", time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	expired, err := s.ClaimExecution(t.Context(), f, a, r, cap)
	if err != nil || expired.State != execution.Cancelled || expired.Steps[0].Reason != "queue_expired" {
		t.Fatal(expired, err)
	}
	if _, err = other.Execution(t.Context(), a); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherUnmanagedAndUncertainRecovery(t *testing.T) {
	s, r, path := reliableStoreFixture(t)
	other := openTest(t, path)
	original, err := s.CreateReliable(t.Context(), r, execution.AdmissionOptions{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	snap := reliableChange(t, s, original.Snapshot, execution.StartStep, time.Now().UTC(), "", "", nil)
	snap = reliableChange(t, s, snap, execution.StartAttempt, time.Now().UTC(), "", "", nil)
	if _, err = s.BeginDispatcher(t.Context(), strings.Repeat("b", 32), false); !errors.Is(err, execution.ErrUnmanaged) {
		t.Fatal(err)
	}
	var generation int
	if err = s.db.QueryRowContext(t.Context(), "SELECT generation FROM dispatcher").Scan(&generation); err != nil || generation != 0 {
		t.Fatal(generation, err)
	}
	f, err := s.BeginDispatcher(t.Context(), strings.Repeat("b", 32), true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverDispatcher(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	recovered, err := other.Execution(t.Context(), snap.ID)
	if err != nil || recovered.State != execution.Failed || recovered.Steps[0].Reason != "uncertain_effect" || recovered.Steps[0].Attempts[0].Reason != "uncertain_effect" {
		t.Fatal(recovered, err)
	}
	if _, err = other.CreateReliable(t.Context(), r, execution.AdmissionOptions{Owner: testOwner}); !errors.Is(err, execution.ErrConflict) {
		t.Fatal("unmanaged admission after ownership migration", err)
	}
	if _, err = s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: execution.DefaultCapacity(), Options: execution.AdmissionOptions{Parent: snap.ID}}); !errors.Is(err, execution.ErrUncertain) {
		t.Fatal(err)
	}
	replay, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: execution.DefaultCapacity(), Options: execution.AdmissionOptions{Parent: snap.ID, AcknowledgedUncertain: true}})
	if err != nil || replay.Snapshot.ID == snap.ID || replay.Snapshot.Parent != snap.ID {
		t.Fatal(replay, err)
	}
	events, err := s.Events(t.Context(), snap.ID)
	if err != nil || events[len(events)-1].Reason != "uncertain_effect" {
		t.Fatal(events, err)
	}
}

func TestDispatcherQueuedCancellationAndArchive(t *testing.T) {
	for _, scenario := range []string{"cancel", "archive"} {
		t.Run(scenario, func(t *testing.T) {
			s, r, _ := reliableStoreFixture(t)
			cap := execution.DefaultCapacity()
			q, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "cancel" {
				if _, err = s.RequestReliableCancellation(t.Context(), q.Snapshot.ID, ""); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = s.Archive(t.Context(), r.Name); err != nil {
					t.Fatal(err)
				}
			}
			f, err := s.BeginDispatcher(t.Context(), testOwner, false)
			if err != nil {
				t.Fatal(err)
			}
			snap, err := s.ClaimExecution(t.Context(), f, q.Snapshot.ID, r, cap)
			if err != nil || snap.State != execution.Cancelled || snap.Deadline != nil {
				t.Fatal(snap, err)
			}
			if _, err = s.RequestReliableCancellation(t.Context(), snap.ID, "first"); err != nil {
				t.Fatal("terminal ack", err)
			}
		})
	}
}

func TestDispatcherAdmissionAndClaimRollback(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	cap := execution.DefaultCapacity()
	if _, err := s.db.ExecContext(t.Context(), "CREATE TRIGGER fail_queue BEFORE INSERT ON execution_queue BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap, Options: execution.AdmissionOptions{IdempotencyKey: "rollback"}}); err == nil {
		t.Fatal("failure hidden")
	}
	for _, table := range []string{"executions", "execution_steps", "execution_attempts", "execution_admissions", "execution_events", "execution_queue"} {
		var count int
		if err := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if _, err := s.db.ExecContext(t.Context(), "DROP TRIGGER fail_queue"); err != nil {
		t.Fatal(err)
	}
	q, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(t.Context(), "CREATE TRIGGER fail_claim BEFORE INSERT ON execution_events WHEN json_extract(NEW.body,'$.state')='running' BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimExecution(t.Context(), f, q.Snapshot.ID, r, cap); err == nil {
		t.Fatal("claim failure hidden")
	}
	snap, err := s.Execution(t.Context(), q.Snapshot.ID)
	if err != nil || snap.State != execution.Queued || snap.Version != 0 || snap.Deadline != nil {
		t.Fatal(snap, err)
	}
}

func TestMigrationFourPreservesHistoryAndRollsBackIndex(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "upgrade", true: "rollback"}[fail], func(t *testing.T) {
			s, r, _ := reliableStoreFixture(t)
			admitted, err := s.CreateReliable(t.Context(), r, execution.AdmissionOptions{Owner: testOwner})
			if err != nil {
				t.Fatal(err)
			}
			snap := admitted.Snapshot
			for _, m := range []execution.ReliableMutation{{Step: "first", Operation: execution.StartStep}, {Step: "first", Operation: execution.StartAttempt}, {Step: "first", Operation: execution.FinishAttempt, Outcome: execution.Succeeded, Output: []byte(`{}`)}, {Operation: execution.FinishExecution}} {
				m.ID, m.Owner, m.Version = snap.ID, testOwner, snap.Version
				snap, err = s.ChangeReliable(t.Context(), m)
				if err != nil {
					t.Fatal(err)
				}
			}
			var before string
			if err = s.db.QueryRowContext(t.Context(), "SELECT body FROM executions WHERE id=?", snap.ID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			for _, statement := range []string{"DROP TABLE event_history", "DROP TABLE event_dedup", "DROP TABLE event_executions", "DROP TABLE startup_scan", "DROP TABLE event_routes", "DROP TABLE schedule_events", "DROP TABLE scheduled_executions", "DROP TABLE schedules", "ALTER TABLE workflows DROP COLUMN automatic_generation", "DROP TABLE dispatcher_audit", "DROP TABLE execution_queue", "DROP TABLE dispatcher", "DELETE FROM schema_migrations WHERE version>=4", "DROP INDEX executions_active_workflow", "CREATE UNIQUE INDEX executions_active_workflow ON executions(workflow_name) WHERE state IN ('pending','queued','running')"} {
				if _, err = s.db.ExecContext(t.Context(), statement); err != nil {
					t.Fatal(err)
				}
			}
			if fail {
				if _, err = s.db.ExecContext(t.Context(), "CREATE TABLE execution_queue(marker TEXT); INSERT INTO execution_queue VALUES('preserved')"); err != nil {
					t.Fatal(err)
				}
			}
			err = s.migrate(t.Context())
			if (err != nil) != fail {
				t.Fatal(err)
			}
			var after string
			if err = s.db.QueryRowContext(t.Context(), "SELECT body FROM executions WHERE id=?", snap.ID).Scan(&after); err != nil || after != before {
				t.Fatal("historical body changed", err)
			}
			if _, err = s.Execution(t.Context(), snap.ID); err != nil {
				t.Fatal(err)
			}
			if fail {
				var sql string
				if err = s.db.QueryRowContext(t.Context(), "SELECT sql FROM sqlite_master WHERE name='executions_active_workflow'").Scan(&sql); err != nil || !strings.Contains(sql, "UNIQUE") {
					t.Fatal(sql, err)
				}
				var count int
				if err = s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 3 {
					t.Fatal(count, err)
				}
			}
		})
	}
}

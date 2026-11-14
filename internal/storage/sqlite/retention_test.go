package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/event"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

func removeMigrationSeven(t *testing.T, s *Store) {
	t.Helper()
	removeMigrationEight(t, s)
	_, err := s.db.ExecContext(t.Context(), `ALTER TABLE dispatcher DROP COLUMN server_state;
DROP TABLE pruned_schedule_occurrences;
DROP INDEX execution_policies_parent;
DROP INDEX execution_admissions_execution;
DROP INDEX event_dedup_execution;
DROP INDEX event_history_execution;
DROP INDEX schedule_events_execution;
DELETE FROM schema_migrations WHERE version=7;`)
	if err != nil {
		t.Fatal(err)
	}
	for table, trigger := range map[string]string{"execution_events": "execution_events_no_delete", "execution_attempts": "attempts_no_delete", "scheduled_executions": "scheduled_executions_no_delete", "schedule_events": "schedule_events_no_delete", "event_executions": "event_executions_no_delete", "event_history": "event_history_no_delete"} {
		_, err := s.db.ExecContext(t.Context(), "DROP TRIGGER "+trigger+"; CREATE TRIGGER "+trigger+" BEFORE DELETE ON "+table+" BEGIN SELECT RAISE(ABORT,'history is immutable'); END;")
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(t.Context(), "DROP TABLE execution_prune_targets"); err != nil {
		t.Fatal(err)
	}
}

func removeMigrationEight(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.db.ExecContext(t.Context(), `ALTER TABLE dispatcher DROP COLUMN worker_capacity;
DROP INDEX execution_attempts_running; DROP INDEX executions_queued;
DELETE FROM schema_migrations WHERE version=8;`); err != nil {
		t.Fatal(err)
	}
}

func ageExecution(t *testing.T, s *Store, id string) {
	t.Helper()
	if _, err := s.db.ExecContext(t.Context(), "UPDATE executions SET body=json_set(body,'$.updatedAt','2020-01-01T00:00:00Z') WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
}

func TestPruneLegacyMigrationAndAtomicRollback(t *testing.T) {
	s, r, path := executionFixture(t)
	old := legacyTerminal(t, s, r)
	ageExecution(t, s, old.ID)
	removeMigrationSeven(t, s)
	if _, err := PreviewPrune(t.Context(), path, time.Hour); err == nil {
		t.Fatal("preview accepted old schema")
	}
	s = openTest(t, path)
	if _, err := s.db.ExecContext(t.Context(), "DELETE FROM execution_events WHERE execution_id=?", old.ID); err == nil {
		t.Fatal("direct deletion of execution events was allowed")
	}
	preview, err := PreviewPrune(t.Context(), path, time.Hour)
	if err != nil || preview != (PruneReport{Eligible: 1}) {
		t.Fatal(preview, err)
	}
	if _, err = s.db.ExecContext(t.Context(), `CREATE TRIGGER fail_prune BEFORE DELETE ON executions BEGIN SELECT RAISE(ABORT,'failure'); END`); err != nil {
		t.Fatal(err)
	}
	if report, err := s.PruneExecutions(t.Context(), time.Hour, false); err == nil || report.Deleted != 0 {
		t.Fatal(report, err)
	}
	if got, err := s.Events(t.Context(), old.ID); err != nil || len(got) == 0 {
		t.Fatal(got, err)
	}
	if _, err = s.db.ExecContext(t.Context(), "DROP TRIGGER fail_prune"); err != nil {
		t.Fatal(err)
	}
	report, err := s.PruneExecutions(t.Context(), time.Hour, false)
	if err != nil || report != (PruneReport{Eligible: 1, Deleted: 1}) {
		t.Fatal(report, err)
	}
	other := openTest(t, path)
	if _, err = other.Execution(t.Context(), old.ID); !errors.Is(err, execution.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err = other.Get(t.Context(), r.Name, r.Revision); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = s.PruneExecutions(ctx, time.Hour, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPruneBatchAndEligibility(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	_, err := s.db.ExecContext(t.Context(), `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<105)
INSERT INTO executions SELECT printf('%032x',x),?,?,CASE WHEN x=105 THEN 'running' ELSE 'failed' END,0,?,
'2020-01-01T00:00:00Z',json_object('updatedAt',CASE WHEN x=104 THEN '9999-01-01T00:00:00Z' ELSE '2020-01-01T00:00:00.123456789Z' END) FROM n;
INSERT INTO execution_policies VALUES(printf('%032x',105),2,16384,printf('%032x',2),0);
INSERT INTO execution_admissions VALUES('scope',printf('%064x',1),printf('%064x',2),printf('%032x',1),'2020-01-01T00:00:00Z','9999-01-01T00:00:00Z');`, r.Name, r.Revision, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	want := PruneReport{Eligible: 101, Active: 1, WithinRetention: 1, Protected: 1, Referenced: 1, Deleted: 100}
	if got, err := s.PruneExecutions(t.Context(), time.Hour, false); err != nil || got != want {
		t.Fatal(got, err)
	}
	want.Eligible, want.Deleted = 1, 1
	if got, err := s.PruneExecutions(t.Context(), time.Hour, false); err != nil || got != want {
		t.Fatal(got, err)
	}
}

func TestPruneAgeAndReceiptBoundary(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	now := time.Date(2026, time.November, 14, 12, 0, 0, 0, time.UTC)
	_, err := s.db.ExecContext(t.Context(), `INSERT INTO executions VALUES
 ('equal',?,?,'succeeded',0,?,'2020-01-01T00:00:00Z','{"updatedAt":"2026-11-14T11:00:00Z"}'),
 ('older',?,?,'succeeded',0,?,'2020-01-01T00:00:00Z','{"updatedAt":"2026-11-14T10:59:59.999999999Z"}');
 INSERT INTO execution_admissions VALUES('scope',printf('%064x',1),printf('%064x',2),'older','2020-01-01T00:00:00Z','2026-11-14T12:00:00.000000001Z');`,
		r.Name, r.Revision, testOwner, r.Name, r.Revision, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.pruneExecutions(t.Context(), time.Hour, false, now); err != nil || got != (PruneReport{WithinRetention: 1, Protected: 1}) {
		t.Fatal(got, err)
	}
	if got, err := s.pruneExecutions(t.Context(), time.Hour, false, now.Add(time.Nanosecond)); err != nil || got != (PruneReport{Eligible: 2, Deleted: 2}) {
		t.Fatal(got, err)
	}
}

func TestPruneConcurrentEventReplayProtectsReceipt(t *testing.T) {
	s, r, path := reliableStoreFixture(t)
	r = dispatcherRevision(t, s, []byte(strings.Replace(string(r.Source), "spec:", "spec:\n  triggers: [{id: build, type: internal, event: build}]", 1)))
	cap := execution.DefaultCapacity()
	request := event.Request{Type: "internal", Name: "build", ID: "retention-test"}
	receipts, err := s.EmitEvent(t.Context(), request, cap, nil)
	if err != nil || len(receipts) != 1 {
		t.Fatal(receipts, err)
	}
	first := receipts[0]
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestReliableCancellation(t.Context(), first.Execution, ""); err != nil {
		t.Fatal(err)
	}
	if snap, err := s.ClaimExecution(t.Context(), f, first.Execution, r, cap); err != nil || snap.State != execution.Cancelled {
		t.Fatal(snap, err)
	}
	ageExecution(t, s, first.Execution)
	other := openTest(t, path)
	result := make(chan error, 1)
	go func() {
		replay, err := other.EmitEvent(t.Context(), request, cap, nil)
		if err == nil && (len(replay) != 1 || replay[0] != first) {
			err = errors.New("replay identity changed")
		}
		result <- err
	}()
	report, err := s.PruneExecutions(t.Context(), time.Hour, false)
	if replayErr := <-result; err != nil || replayErr != nil || report != (PruneReport{Protected: 1}) {
		t.Fatal(report, err, replayErr)
	}
	if _, err = s.db.ExecContext(t.Context(), "UPDATE event_dedup SET expires_at=0"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.PruneExecutions(t.Context(), time.Hour, false); err != nil || got.Deleted != 1 {
		t.Fatal(got, err)
	}
	if replay, err := other.EmitEvent(t.Context(), request, cap, nil); err != nil || len(replay) != 1 || replay[0] == first {
		t.Fatal(replay, err)
	}
}

func TestPrunePreservesScheduledOccurrenceAcrossRestart(t *testing.T) {
	s, r, f, now, path := scheduledFixture(t)
	cap := execution.DefaultCapacity()
	if report, err := scheduleService(s).Tick(t.Context(), f, cap, now); err != nil || report.Admitted != 1 {
		t.Fatal(report, err)
	}
	rows, err := s.Executions(t.Context())
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	id := rows[0].ID
	if _, err = s.RequestReliableCancellation(t.Context(), id, ""); err != nil {
		t.Fatal(err)
	}
	if snap, err := s.ClaimExecution(t.Context(), f, id, r, cap); err != nil || snap.State != execution.Cancelled {
		t.Fatal(snap, err)
	}
	ageExecution(t, s, id)
	if report, err := s.PruneExecutions(t.Context(), time.Hour, false); err != nil || report.Deleted != 1 {
		t.Fatal(report, err)
	}
	reopened := openTest(t, path)
	cursor := now.Truncate(time.Minute).Add(-time.Minute)
	if _, err = reopened.db.ExecContext(t.Context(), "UPDATE schedules SET cursor_at=?,checked_minute=?,retry_at=0,version=version+1", cursor.Format(time.RFC3339Nano), cursor.Unix()); err != nil {
		t.Fatal(err)
	}
	f, err = reopened.BeginDispatcher(t.Context(), strings.Repeat("b", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := scheduleService(reopened).Tick(t.Context(), f, cap, now); err != nil || report.Admitted != 0 {
		t.Fatal(report, err)
	}
	if rows, err := reopened.Executions(t.Context()); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}

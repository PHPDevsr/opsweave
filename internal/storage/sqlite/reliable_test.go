package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

// Future-policy revisions are installed as isolated persistence fixtures. The
// production registration/CLI capability gate is deliberately unchanged.
func reliableStoreFixture(t *testing.T) (*Store, registry.Record, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reliable.sqlite")
	s := openTest(t, path)
	source := []byte("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: reliable}\nspec:\n  steps:\n    - id: first\n      uses: test.echo\n      with: {message: hello}\n      retry: {attempts: 3, backoff: 1s, maxBackoff: 2s}\n")
	definition, err := workflow.Parse(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := registry.Normalize(definition)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := applyOne(t.Context(), tx, registry.Candidate{Name: "reliable", Source: source, Normalized: normalized, Bindings: map[string]string{"test.echo": "1"}}, nil)
	if err != nil {
		t.Fatal(errors.Join(err, tx.Rollback()))
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return s, record, path
}

func reliableChange(t *testing.T, s *Store, snap execution.Snapshot, op execution.Operation, at time.Time, outcome execution.State, reason string, output json.RawMessage) execution.Snapshot {
	t.Helper()
	step := "first"
	if op == execution.FinishExecution {
		step = ""
	}
	next, err := s.changeReliable(t.Context(), execution.ReliableMutation{ID: snap.ID, Owner: testOwner, Version: snap.Version, Step: step, Operation: op, Outcome: outcome, Reason: reason, Output: output}, func() time.Time { return at })
	if err != nil {
		t.Fatal(op, err)
	}
	return next
}

func TestReliableAttemptHistoryRestartAndImmutability(t *testing.T) {
	s, record, path := reliableStoreFixture(t)
	admitted, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner, IdempotencyKey: "never-store-plaintext"})
	if err != nil || !admitted.Created {
		t.Fatal(admitted, err)
	}
	snap := admitted.Snapshot
	at := snap.CreatedAt
	snap = reliableChange(t, s, snap, execution.StartStep, at, "", "", nil)
	snap = reliableChange(t, s, snap, execution.StartAttempt, at, "", "", nil)
	snap = reliableChange(t, s, snap, execution.FinishAttempt, at, execution.Failed, "action_failed", nil)
	if len(snap.Steps[0].Attempts) != 1 || snap.Steps[0].State != execution.Running {
		t.Fatal(snap.Steps)
	}
	snap = reliableChange(t, s, snap, execution.StartAttempt, at.Add(time.Second), "", "", nil)
	snap = reliableChange(t, s, snap, execution.FinishAttempt, at.Add(time.Second), execution.Succeeded, "", json.RawMessage(`{"message":"ok"}`))
	snap = reliableChange(t, s, snap, execution.FinishExecution, at.Add(time.Second), "", "", nil)
	reopened := openTest(t, path)
	got, err := reopened.Execution(t.Context(), snap.ID)
	if err != nil || got.State != execution.Succeeded || got.Format != 2 || len(got.Steps[0].Attempts) != 2 || got.Steps[0].Attempts[0].Reason != "action_failed" {
		t.Fatal(got, err)
	}
	events, err := s.Events(t.Context(), snap.ID)
	if err != nil || len(events) != 11 {
		t.Fatal(len(events), err)
	}
	codes := []string{}
	for _, event := range events {
		codes = append(codes, event.Code)
	}
	if !reflect.DeepEqual(codes, []string{"execution_state", "execution_state", "execution_state", "step_state", "attempt_started", "attempt_finished", "retry_scheduled", "attempt_started", "attempt_finished", "step_state", "execution_state"}) {
		t.Fatal(codes)
	}
	for _, statement := range []string{"UPDATE execution_attempts SET body='{}' WHERE number=1", "DELETE FROM execution_attempts"} {
		if _, err := s.db.ExecContext(t.Context(), statement); err == nil {
			t.Fatal("mutable terminal attempt")
		}
	}
	var digest string
	if err := s.db.QueryRowContext(t.Context(), "SELECT digest FROM execution_admissions").Scan(&digest); err != nil || len(digest) != 64 || strings.Contains(digest, "plaintext") {
		t.Fatal(digest, err)
	}
	if _, err := s.Change(t.Context(), execution.Mutation{ID: snap.ID, Owner: testOwner, Version: snap.Version, To: execution.Failed}); err == nil {
		t.Fatal("legacy mutation bypassed format 2")
	}
}

func TestReliableTransactionFailuresRollBackAllHistory(t *testing.T) {
	s, record, _ := reliableStoreFixture(t)
	trigger := `CREATE TRIGGER reliable_fault BEFORE INSERT ON execution_events BEGIN SELECT RAISE(ABORT,'injected fault'); END`
	if _, err := s.db.ExecContext(t.Context(), trigger); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner, IdempotencyKey: "key"}); err == nil {
		t.Fatal("admission fault ignored")
	}
	for _, table := range []string{"executions", "execution_steps", "execution_policies", "execution_attempts", "execution_admissions"} {
		var count int
		if err := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if _, err := s.db.ExecContext(t.Context(), "DROP TRIGGER reliable_fault"); err != nil {
		t.Fatal(err)
	}
	admitted, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	snap := admitted.Snapshot
	snap = reliableChange(t, s, snap, execution.StartStep, snap.CreatedAt, "", "", nil)
	snap = reliableChange(t, s, snap, execution.StartAttempt, snap.CreatedAt, "", "", nil)
	if _, err := s.db.ExecContext(t.Context(), trigger); err != nil {
		t.Fatal(err)
	}
	_, err = s.ChangeReliable(t.Context(), execution.ReliableMutation{ID: snap.ID, Owner: testOwner, Version: snap.Version, Step: "first", Operation: execution.FinishAttempt, Outcome: execution.Failed, Reason: "action_failed"})
	if err == nil {
		t.Fatal("attempt commit fault ignored")
	}
	got, err := s.Execution(t.Context(), snap.ID)
	if err != nil || got.Version != snap.Version || got.Steps[0].Attempts[0].State != execution.Running {
		t.Fatal(got, err)
	}
	events, err := s.Events(t.Context(), snap.ID)
	if err != nil || len(events) != 5 {
		t.Fatal(len(events), err)
	}
}

func TestReliableCancellationAcknowledgesRequestOnly(t *testing.T) {
	s, record, _ := reliableStoreFixture(t)
	a, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	snap := a.Snapshot
	snap = reliableChange(t, s, snap, execution.StartStep, snap.CreatedAt, "", "", nil)
	snap = reliableChange(t, s, snap, execution.StartAttempt, snap.CreatedAt, "", "", nil)
	requested, err := s.RequestReliableCancellation(t.Context(), snap.ID, "first")
	if err != nil || requested.State != execution.Running || requested.Steps[0].CancelRequestedAt == nil {
		t.Fatal(requested, err)
	}
	again, err := s.RequestReliableCancellation(t.Context(), snap.ID, "first")
	if err != nil || again.Version != requested.Version {
		t.Fatal(again, err)
	}
	if _, err := s.ChangeReliable(t.Context(), execution.ReliableMutation{ID: snap.ID, Owner: testOwner, Version: snap.Version, Step: "first", Operation: execution.FinishAttempt, Outcome: execution.Succeeded, Output: json.RawMessage(`{}`)}); err != execution.ErrConflict {
		t.Fatal(err)
	}
	requested = reliableChange(t, s, requested, execution.FinishAttempt, requested.UpdatedAt, execution.Succeeded, "", json.RawMessage(`{"discard":"result"}`))
	if requested.Steps[0].State != execution.Cancelled || len(requested.Steps[0].Output) != 0 {
		t.Fatal(requested.Steps)
	}
	requested = reliableChange(t, s, requested, execution.FinishExecution, requested.UpdatedAt, "", "", nil)
	unchanged, err := s.RequestReliableCancellation(t.Context(), snap.ID, "")
	if err != nil || unchanged.Version != requested.Version || unchanged.State != execution.Cancelled {
		t.Fatal(unchanged, err)
	}
}

func TestReliableConcurrentIdempotencyAndConflict(t *testing.T) {
	s, record, path := reliableStoreFixture(t)
	other := openTest(t, path)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan execution.AdmissionResult, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for _, store := range []*Store{s, other} {
		wg.Go(func() {
			<-start
			result, err := store.CreateReliable(ctx, record, execution.AdmissionOptions{Owner: testOwner, IdempotencyKey: "same-key"})
			results <- result
			failures <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	close(failures)
	created := 0
	id := ""
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if result.Created {
			created++
		}
		if id != "" && id != result.Snapshot.ID {
			t.Fatal("duplicate execution")
		}
		id = result.Snapshot.ID
	}
	if created != 1 {
		t.Fatal(created)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM executions").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err := s.CreateReliable(ctx, record, execution.AdmissionOptions{Owner: testOwner, IdempotencyKey: "same-key", AcknowledgedUncertain: true}); err != execution.ErrConflict {
		t.Fatal(err)
	}
}

func TestReliableWholeReplayPinsOldRevision(t *testing.T) {
	s, record, _ := reliableStoreFixture(t)
	a, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	snap := a.Snapshot
	cancelled, err := s.RequestReliableCancellation(t.Context(), snap.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	cancelled = reliableChange(t, s, cancelled, execution.CancelIdleStep, cancelled.UpdatedAt, "", "", nil)
	cancelled = reliableChange(t, s, cancelled, execution.FinishExecution, cancelled.UpdatedAt, "", "", nil)
	// Publish another immutable revision through the fixture's validated source path.
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := applyOne(t.Context(), tx, registry.Candidate{Name: record.Name, Source: record.Source, Normalized: record.Definition, Bindings: map[string]string{"test.echo": "1"}}, nil)
	if err != nil {
		t.Fatal(errors.Join(err, tx.Rollback()))
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	replay, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner, Parent: snap.ID})
	if err != nil || !replay.Created || replay.Snapshot.ID == snap.ID || replay.Snapshot.Parent != snap.ID || replay.Snapshot.Revision != record.Revision || replay.Snapshot.Revision == replacement.Revision || replay.Snapshot.Steps[0].Attempt != 0 {
		t.Fatal(replay, err)
	}
	original, err := s.Execution(t.Context(), snap.ID)
	if err != nil || original.State != execution.Cancelled || original.Version != cancelled.Version {
		t.Fatal(original, err)
	}
}

func TestReliableEventCapacityAndLegacyUpgrade(t *testing.T) {
	s, record, path := reliableStoreFixture(t)
	a, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 1022 {
		if err := appendEvent(t.Context(), tx, execution.Event{Execution: a.Snapshot.ID, Time: a.Snapshot.CreatedAt, Code: "execution_state", State: execution.Running}); err != nil {
			t.Fatal(errors.Join(err, tx.Rollback()))
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events(t.Context(), a.Snapshot.ID)
	if err != nil || len(events) != 1025 {
		t.Fatal(len(events), err)
	}
	reopened := openTest(t, path)
	if _, err := reopened.Execution(t.Context(), a.Snapshot.ID); err != nil {
		t.Fatal(err)
	}
}

func versionTwoFixture(t *testing.T, s *Store) {
	t.Helper()
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		"DROP TABLE schedule_events", "DROP TABLE scheduled_executions", "DROP TABLE schedules", "ALTER TABLE workflows DROP COLUMN automatic_generation", "DROP TABLE dispatcher_audit", "DROP TABLE execution_queue", "DROP TABLE dispatcher", "DROP TABLE execution_admissions", "DROP TABLE execution_attempts", "DROP TABLE execution_policies",
		"DROP TRIGGER execution_events_no_update", "DROP TRIGGER execution_events_no_delete",
		"ALTER TABLE execution_events RENAME TO fixture_events",
		`CREATE TABLE execution_events(execution_id TEXT NOT NULL REFERENCES executions(id),sequence INTEGER NOT NULL CHECK(sequence>0 AND sequence<=1024),body TEXT NOT NULL,PRIMARY KEY(execution_id,sequence))`,
		"INSERT INTO execution_events SELECT * FROM fixture_events", "DROP TABLE fixture_events",
		`CREATE TRIGGER execution_events_no_update BEFORE UPDATE ON execution_events BEGIN SELECT RAISE(ABORT,'execution events are immutable'); END`,
		`CREATE TRIGGER execution_events_no_delete BEFORE DELETE ON execution_events BEGIN SELECT RAISE(ABORT,'execution events are immutable'); END`,
		"DELETE FROM schema_migrations WHERE version>=3",
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(errors.Join(err, tx.Rollback()))
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func legacyTerminal(t *testing.T, s *Store, record registry.Record) execution.Snapshot {
	t.Helper()
	snap, err := s.Submit(t.Context(), record, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []execution.Mutation{{Step: "first", To: execution.Running}, {Step: "first", To: execution.Succeeded, Output: json.RawMessage(`{"value":"original"}`)}, {To: execution.Succeeded}} {
		m.ID, m.Owner, m.Version = snap.ID, testOwner, snap.Version
		snap, err = s.Change(t.Context(), m)
		if err != nil {
			t.Fatal(err)
		}
	}
	return snap
}

func TestMigrationThreePreservesLegacyHistory(t *testing.T) {
	s, record, path := executionFixture(t)
	original := legacyTerminal(t, s, record)
	events, err := s.Events(t.Context(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	versionTwoFixture(t, s)
	upgraded := openTest(t, path)
	after, err := upgraded.Execution(t.Context(), original.ID)
	if err != nil || !reflect.DeepEqual(original, after) {
		t.Fatal(after, err)
	}
	afterEvents, err := upgraded.Events(t.Context(), original.ID)
	if err != nil || !reflect.DeepEqual(events, afterEvents) {
		t.Fatal(afterEvents, err)
	}
	next, err := upgraded.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner})
	if err != nil || next.Snapshot.Format != 2 {
		t.Fatal(next, err)
	}
	var legacyLimit int
	if err := upgraded.db.QueryRowContext(t.Context(), "SELECT COALESCE(MAX(event_limit),1024) FROM execution_policies WHERE execution_id=?", original.ID).Scan(&legacyLimit); err != nil || legacyLimit != 1024 {
		t.Fatal(legacyLimit, err)
	}
}

func TestMigrationThreeFailureRestoresLegacyGuards(t *testing.T) {
	s, record, path := executionFixture(t)
	snap := legacyTerminal(t, s, record)
	versionTwoFixture(t, s)
	if _, err := s.db.ExecContext(t.Context(), "CREATE TABLE execution_policies(marker TEXT)"); err != nil {
		t.Fatal(err)
	}
	next, err := Open(t.Context(), path)
	if err == nil {
		if closeErr := next.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal("conflicting migration accepted")
	}
	var sql string
	var count int
	if err := s.db.QueryRowContext(t.Context(), "SELECT sql FROM sqlite_master WHERE name='execution_events'").Scan(&sql); err != nil || !strings.Contains(sql, "1024") {
		t.Fatal(sql, err)
	}
	if err := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	if _, err := s.db.ExecContext(t.Context(), "DELETE FROM execution_events WHERE execution_id=?", snap.ID); err == nil {
		t.Fatal("rollback lost immutable-event trigger")
	}
	var body string
	if err := s.db.QueryRowContext(t.Context(), "SELECT body FROM executions WHERE id=?", snap.ID).Scan(&body); err != nil || !strings.Contains(body, "succeeded") {
		t.Fatal(body, err)
	}
}

func TestReliableAdmissionExpiryAndUncertainAcknowledgement(t *testing.T) {
	s, record, _ := reliableStoreFixture(t)
	options := execution.AdmissionOptions{Owner: testOwner, IdempotencyKey: "expiring"}
	a, err := s.CreateReliable(t.Context(), record, options)
	if err != nil {
		t.Fatal(err)
	}
	var created, expires string
	if err := s.db.QueryRowContext(t.Context(), "SELECT created_at,expires_at FROM execution_admissions").Scan(&created, &expires); err != nil {
		t.Fatal(err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || expiresAt.Sub(createdAt) != 7*24*time.Hour {
		t.Fatal(expires, err)
	}
	cancelled, err := s.RequestReliableCancellation(t.Context(), a.Snapshot.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	cancelled = reliableChange(t, s, cancelled, execution.CancelIdleStep, cancelled.UpdatedAt, "", "", nil)
	cancelled = reliableChange(t, s, cancelled, execution.FinishExecution, cancelled.UpdatedAt, "", "", nil)
	// Simulate durable uncertain-effect metadata from the later recovery adapter.
	step := cancelled.Steps[0]
	step.Reason = "uncertain_effect"
	body, err := stepBody(step)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), "UPDATE execution_steps SET body=? WHERE execution_id=?", body, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner, Parent: cancelled.ID}); err != execution.ErrUncertain {
		t.Fatal(err)
	}
	if _, err := s.RequestReliableCancellation(t.Context(), cancelled.ID, "unknown"); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	// Expired admission may create work again; unexpired reads do not extend expiry.
	if _, err := s.db.ExecContext(t.Context(), "UPDATE execution_admissions SET expires_at=?", createdAt.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateReliable(t.Context(), record, options)
	if err != nil || !b.Created || b.Snapshot.ID == a.Snapshot.ID {
		t.Fatal(b, err)
	}
	next, err := s.RequestReliableCancellation(t.Context(), b.Snapshot.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	next = reliableChange(t, s, next, execution.CancelIdleStep, next.UpdatedAt, "", "", nil)
	next = reliableChange(t, s, next, execution.FinishExecution, next.UpdatedAt, "", "", nil)
	acknowledged, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner, Parent: cancelled.ID, AcknowledgedUncertain: true})
	if err != nil || !acknowledged.Created || !acknowledged.Snapshot.AcknowledgedUncertain {
		t.Fatal(acknowledged, err)
	}
	if _, err := s.Archive(t.Context(), record.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner}); err != registry.ErrArchived {
		t.Fatal(err)
	}
}

func TestReliableOwnershipControlAndRequestErrors(t *testing.T) {
	s, record, _ := reliableStoreFixture(t)
	admitted, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	id := admitted.Snapshot.ID
	other := strings.Repeat("f", 32)
	if _, err := s.OwnedReliable(t.Context(), id, other); err != execution.ErrConflict {
		t.Fatal(err)
	}
	if _, err := s.ReliableControl(t.Context(), id, other); err != execution.ErrConflict {
		t.Fatal(err)
	}
	if _, err := s.ChangeReliable(t.Context(), execution.ReliableMutation{ID: id, Owner: other, Step: "first", Operation: execution.StartStep}); err != execution.ErrConflict {
		t.Fatal(err)
	}
	if _, err := s.RequestReliableCancellation(t.Context(), id, "unknown"); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	control, err := s.ReliableControl(t.Context(), id, testOwner)
	if err != nil || control.Execution || len(control.Steps) != 0 {
		t.Fatal(control, err)
	}
	request, err := s.RequestReliableCancellation(t.Context(), id, "first")
	if err != nil {
		t.Fatal(err)
	}
	control, err = s.ReliableControl(t.Context(), id, testOwner)
	if err != nil || !control.Steps["first"] || control.Execution {
		t.Fatal(control, err)
	}
	owned, err := s.OwnedReliable(t.Context(), id, testOwner)
	if err != nil || owned.Version != request.Version {
		t.Fatal(owned, err)
	}
	if _, err := s.RequestReliableCancellation(t.Context(), id, ""); err != nil {
		t.Fatal(err)
	}
	control, err = s.ReliableControl(t.Context(), id, testOwner)
	if err != nil || !control.Execution {
		t.Fatal(control, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.OwnedReliable(ctx, id, testOwner); err == nil {
		t.Fatal("cancelled control read succeeded")
	}
	for _, invalid := range []string{"", "bad"} {
		if _, err := s.OwnedReliable(t.Context(), invalid, testOwner); err != execution.ErrInvalid {
			t.Fatal(err)
		}
		if _, err := s.ReliableControl(t.Context(), id, invalid); err != execution.ErrInvalid {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: "bad"}); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.OwnedReliable(t.Context(), other, testOwner); err != execution.ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.ReliableControl(t.Context(), other, testOwner); err != execution.ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.RequestReliableCancellation(t.Context(), other, ""); err != execution.ErrNotFound {
		t.Fatal(err)
	}
}

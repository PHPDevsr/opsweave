package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
)

const testOwner = "0123456789abcdef0123456789abcdef"

func executionFixture(t *testing.T) (*Store, registry.Record, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	s := openTest(t, path)
	records, err := s.Apply(t.Context(), []registry.Candidate{candidate(t, "alpha", "hello")})
	if err != nil {
		t.Fatal(err)
	}
	return s, records[0], path
}

func TestExecutionAtomicityAndOwnership(t *testing.T) {
	s, record, _ := executionFixture(t)
	snap, err := s.Submit(t.Context(), record, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	change := func(step string, to execution.State, reason string) (execution.Snapshot, error) {
		return s.Change(t.Context(), execution.Mutation{ID: snap.ID, Owner: testOwner, Version: snap.Version, Step: step, To: to, Reason: reason})
	}
	if _, err := s.Change(t.Context(), execution.Mutation{ID: snap.ID, Owner: strings.Repeat("f", 32), Step: "first", To: execution.Running}); err != execution.ErrConflict {
		t.Fatal(err)
	}
	if _, err := change("", execution.Succeeded, ""); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), `CREATE TRIGGER reject_event BEFORE INSERT ON execution_events BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := change("first", execution.Running, ""); err == nil {
		t.Fatal("fault ignored")
	}
	got, err := s.Execution(t.Context(), snap.ID)
	if err != nil || got.Version != 0 || got.Steps[0].State != execution.Pending {
		t.Fatal(got, err)
	}
	events, err := s.Events(t.Context(), snap.ID)
	if err != nil || len(events) != 3 {
		t.Fatal(events, err)
	}
	if _, err := s.db.ExecContext(t.Context(), "DROP TRIGGER reject_event"); err != nil {
		t.Fatal(err)
	}
	snap, err = change("first", execution.Running, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Change(t.Context(), execution.Mutation{ID: snap.ID, Owner: testOwner, Version: 0, Step: "first", To: execution.Failed, Reason: "action_failed"}); err != execution.ErrConflict {
		t.Fatal(err)
	}
	snap, err = change("first", execution.Succeeded, "")
	if err != nil {
		t.Fatal(err)
	}
	snap, err = change("", execution.Succeeded, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := change("", execution.Failed, ""); err != execution.ErrConflict {
		t.Fatal(err)
	}
	for _, statement := range []string{"UPDATE execution_events SET body='{}'", "DELETE FROM execution_events"} {
		if _, err := s.db.ExecContext(t.Context(), statement); err == nil {
			t.Fatal("mutable events")
		}
	}
	if _, err := s.Submit(t.Context(), record, testOwner); err != nil {
		t.Fatal("terminal run blocked submission", err)
	}
}

func TestSubmissionRollbackAndRevisionConflict(t *testing.T) {
	s, record, _ := executionFixture(t)
	if _, err := s.db.ExecContext(t.Context(), `CREATE TRIGGER reject_event BEFORE INSERT ON execution_events BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(t.Context(), record, testOwner); err == nil {
		t.Fatal("submit fault ignored")
	}
	list, err := s.Executions(t.Context())
	if err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	if _, err := s.db.ExecContext(t.Context(), "DROP TRIGGER reject_event"); err != nil {
		t.Fatal(err)
	}
	forged := record
	forged.Source = []byte("forged")
	if _, err := s.Submit(t.Context(), forged, testOwner); err != execution.ErrConflict {
		t.Fatal(err)
	}
	if _, err := s.Apply(t.Context(), []registry.Candidate{candidate(t, "alpha", "updated")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(t.Context(), record, testOwner); err != execution.ErrConflict {
		t.Fatal(err)
	}
	current, err := s.Get(t.Context(), "alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(t.Context(), "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(t.Context(), current, testOwner); err != registry.ErrArchived {
		t.Fatal(err)
	}
}

func TestConcurrentSubmissionAndRestart(t *testing.T) {
	s, record, path := executionFixture(t)
	other := openTest(t, path)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	ids := make(chan string, 2)
	var wg sync.WaitGroup
	for _, store := range []*Store{s, other} {
		wg.Go(func() {
			<-start
			snap, err := store.Submit(ctx, record, testOwner)
			if err == nil {
				ids <- snap.ID
			}
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, execution.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	id := <-ids
	reopened := openTest(t, path)
	snap, err := reopened.Execution(ctx, id)
	if err != nil || snap.State != execution.Running || snap.Revision != record.Revision {
		t.Fatal(snap, err)
	}
	if _, err := reopened.Submit(ctx, record, testOwner); err != execution.ErrConflict {
		t.Fatal("restart replayed unfinished execution", err)
	}
}

func TestExecutionReadAndRequestErrors(t *testing.T) {
	s, record, _ := executionFixture(t)
	if _, err := s.Submit(t.Context(), record, "bad"); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.Execution(t.Context(), "bad"); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.Events(t.Context(), "bad"); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.Change(t.Context(), execution.Mutation{}); err != execution.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.Execution(t.Context(), testOwner); err != execution.ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.Events(t.Context(), testOwner); err != execution.ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.Change(t.Context(), execution.Mutation{ID: testOwner, Owner: testOwner}); err != execution.ErrNotFound {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Submit(ctx, record, testOwner); err == nil {
		t.Fatal("cancelled submission")
	}
	snap, err := s.Submit(t.Context(), record, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"execution_steps", "executions"} {
		var original string
		query := "SELECT body FROM " + table
		if err := s.db.QueryRowContext(t.Context(), query).Scan(&original); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(t.Context(), "UPDATE "+table+" SET body='{}'"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Execution(t.Context(), snap.ID); err == nil {
			t.Fatal("corrupt snapshot accepted")
		}
		if _, err := s.db.ExecContext(t.Context(), "UPDATE "+table+" SET body=?", original); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpgradeFoundationPreservesRevisions(t *testing.T) {
	s, record, path := executionFixture(t)
	// Construct an actual version-1 database with existing workflow data, then reopen with all current migrations.
	for _, sql := range []string{"DROP TABLE schedule_events", "DROP TABLE scheduled_executions", "DROP TABLE schedules", "ALTER TABLE workflows DROP COLUMN automatic_generation", "DROP TABLE dispatcher_audit", "DROP TABLE execution_queue", "DROP TABLE dispatcher", "DROP TABLE execution_admissions", "DROP TABLE execution_attempts", "DROP TABLE execution_policies", "DROP TABLE execution_events", "DROP TABLE execution_steps", "DROP TABLE executions", "DELETE FROM schema_migrations WHERE version>1"} {
		if _, err := s.db.ExecContext(t.Context(), sql); err != nil {
			t.Fatal(err)
		}
	}
	upgraded := openTest(t, path)
	after, err := upgraded.Get(t.Context(), record.Name, record.Revision)
	if err != nil || string(after.Source) != string(record.Source) {
		t.Fatal(after, err)
	}
	snap, err := upgraded.Submit(t.Context(), after, testOwner)
	if err != nil || snap.Revision != record.Revision {
		t.Fatal(snap, err)
	}
	var count int
	if err := upgraded.db.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 5 {
		t.Fatal(count, err)
	}
}

func TestExecutionListLimitReturnsNoPartialResult(t *testing.T) {
	s, record, _ := executionFixture(t)
	body := execution.Snapshot{ID: testOwner, Workflow: record.Name, Revision: record.Revision, State: execution.Succeeded, CreatedAt: time.Now().UTC()}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.ExecContext(t.Context(), `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10001) INSERT INTO executions SELECT printf('%032x',x),?,?,'succeeded',0,?,'2026-11-08T00:00:00Z',? FROM n`, record.Name, record.Revision, testOwner, string(data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.Executions(t.Context())
	if !errors.Is(err, execution.ErrLimit) || rows != nil {
		t.Fatal(len(rows), err)
	}
}

func TestExecutionEventLimitRollsBackState(t *testing.T) {
	s, record, _ := executionFixture(t)
	snap, err := s.Submit(t.Context(), record, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.ExecContext(t.Context(), `WITH RECURSIVE n(x) AS (VALUES(4) UNION ALL SELECT x+1 FROM n WHERE x<1024) INSERT INTO execution_events SELECT ?,x,'{}' FROM n`, snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Change(t.Context(), execution.Mutation{ID: snap.ID, Owner: testOwner, Step: "first", To: execution.Running}); !errors.Is(err, execution.ErrLimit) {
		t.Fatal(err)
	}
	got, err := s.Execution(t.Context(), snap.ID)
	if err != nil || got.Version != 0 || got.Steps[0].State != execution.Pending {
		t.Fatal(got, err)
	}
}

func TestExecutionMigrationFailurePreservesFoundation(t *testing.T) {
	s, record, path := executionFixture(t)
	for _, statement := range []string{"DROP TABLE schedule_events", "DROP TABLE scheduled_executions", "DROP TABLE schedules", "ALTER TABLE workflows DROP COLUMN automatic_generation", "DROP TABLE dispatcher_audit", "DROP TABLE execution_queue", "DROP TABLE dispatcher", "DROP TABLE execution_admissions", "DROP TABLE execution_attempts", "DROP TABLE execution_policies", "DROP TABLE execution_events", "DROP TABLE execution_steps", "DROP TABLE executions", "DELETE FROM schema_migrations WHERE version>1", "CREATE TABLE executions(marker TEXT)", "INSERT INTO executions VALUES('preserved')"} {
		if _, err := s.db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if next, err := Open(t.Context(), path); err == nil {
		if closeErr := next.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal("conflicting v2 migration accepted")
	}
	after, err := s.Get(t.Context(), record.Name, record.Revision)
	if err != nil || string(after.Source) != string(record.Source) {
		t.Fatal(after, err)
	}
	var count int
	var marker string
	if err := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err := s.db.QueryRowContext(t.Context(), "SELECT marker FROM executions").Scan(&marker); err != nil || marker != "preserved" {
		t.Fatal(marker, err)
	}
}

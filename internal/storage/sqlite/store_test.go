package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/registry"
	driver "modernc.org/sqlite"
)

func openTest(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func candidate(t *testing.T, name, message string) registry.Candidate {
	t.Helper()
	source := "# retain this comment\napiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: " + name + "}\nspec:\n  steps:\n    - id: first\n      uses: test.echo\n      with: {message: " + message + "}\n"
	catalog := registry.Catalog{"test.echo": {ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"message": {Type: "string", Required: true}}}}}
	result, err := registry.Prepare(t.Context(), []registry.Source{{Data: []byte(source)}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return result[0]
}

func TestRegistryPersistenceAndHistory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	s := openTest(t, path)
	if err := s.Health(t.Context()); err != nil {
		t.Fatal(err)
	}
	first := candidate(t, "alpha", "one")
	created, err := s.Apply(t.Context(), []registry.Candidate{first})
	if err != nil || len(created) != 1 || created[0].Number != 1 || !created[0].Enabled {
		t.Fatalf("apply = %v, %v", created, err)
	}
	id := created[0].Revision
	if len(id) != 32 {
		t.Fatal("invalid opaque revision identity")
	}
	if _, err := s.SetEnabled(t.Context(), "alpha", false); err != nil {
		t.Fatal(err)
	}
	updated, err := s.Apply(t.Context(), []registry.Candidate{candidate(t, "alpha", "two")})
	if err != nil || updated[0].Number != 2 || updated[0].Enabled || updated[0].Revision == id {
		t.Fatalf("update = %v, %v", updated, err)
	}
	old, err := s.Get(t.Context(), "alpha", id)
	if err != nil || !bytes.Equal(old.Source, first.Source) || !bytes.Equal(old.Definition, first.Normalized) {
		t.Fatalf("historical revision changed: %v", err)
	}
	old.Source[0] = 'x'
	readAgain, err := s.Get(t.Context(), "alpha", id)
	if err != nil || !bytes.Equal(readAgain.Source, first.Source) {
		t.Fatal("read values alias stored state")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, path)
	current, err := s.Get(t.Context(), "alpha", "")
	if err != nil || current.Number != 2 || current.Enabled {
		t.Fatalf("restart lost state: %+v, %v", current, err)
	}
	history, err := s.Revisions(t.Context(), "alpha")
	if err != nil || len(history) != 2 || history[0].Revision != id {
		t.Fatalf("history = %v, %v", history, err)
	}
	if _, err := s.SetEnabled(t.Context(), "alpha", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(t.Context(), "alpha"); err != nil {
		t.Fatal(err)
	}
	for _, action := range []func() error{
		func() error { _, err := s.SetEnabled(t.Context(), "alpha", true); return err },
		func() error { _, err := s.Apply(t.Context(), []registry.Candidate{first}); return err },
	} {
		if err := action(); !errors.Is(err, registry.ErrArchived) {
			t.Fatalf("archived name not reserved: %v", err)
		}
	}
	list, err := s.List(t.Context(), false)
	if err != nil || len(list) != 0 {
		t.Fatalf("archived visible in normal list: %v", err)
	}
	list, err = s.List(t.Context(), true)
	if err != nil || len(list) != 1 || !list[0].Archived || list[0].Enabled {
		t.Fatalf("archived state = %v, %v", list, err)
	}
	if _, err := s.Get(t.Context(), "alpha", id); err != nil {
		t.Fatal("archiving lost history", err)
	}
	if _, err := s.Get(t.Context(), "other", id); !errors.Is(err, registry.ErrNotFound) {
		t.Fatal("revision readable through wrong workflow", err)
	}
}

func TestBatchRollbackAndRevisionConstraints(t *testing.T) {
	t.Parallel()
	s := openTest(t, filepath.Join(t.TempDir(), "db.sqlite"))
	a := candidate(t, "alpha", "one")
	b := candidate(t, "beta", "two")
	rows, err := s.Apply(t.Context(), []registry.Candidate{a})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(t.Context(), "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(t.Context(), []registry.Candidate{b, a}); !errors.Is(err, registry.ErrArchived) {
		t.Fatal(err)
	}
	if _, err := s.Get(t.Context(), "beta", ""); !errors.Is(err, registry.ErrNotFound) {
		t.Fatal("failed batch left partial workflow", err)
	}
	for _, query := range []string{
		"UPDATE workflow_revisions SET normalized='{}'",
		"DELETE FROM workflow_revisions",
	} {
		if _, err := s.db.ExecContext(t.Context(), query); err == nil {
			t.Fatal("immutable revision changed")
		}
	}
	if _, err := s.Apply(t.Context(), []registry.Candidate{b}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), "UPDATE workflows SET current_revision=? WHERE name='beta'", rows[0].Revision); err == nil {
		t.Fatal("cross-workflow current revision allowed")
	}
	bad := b
	bad.Normalized = []byte(`{"tampered":true}`)
	if _, err := s.Apply(t.Context(), []registry.Candidate{bad}); err == nil {
		t.Fatal("invalid normalized definition accepted")
	}
	if _, err := s.Apply(t.Context(), []registry.Candidate{b, b}); err == nil {
		t.Fatal("duplicate batch accepted")
	}
	if _, err := s.Apply(t.Context(), nil); err == nil {
		t.Fatal("empty batch accepted")
	}
}

func TestConcurrentApplyAndCancellation(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	a, b := openTest(t, path), openTest(t, path)
	c := candidate(t, "alpha", "one")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	start := make(chan struct{})
	errorsCh := make(chan error, 12)
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			store := a
			if i%2 != 0 {
				store = b
			}
			_, err := store.Apply(ctx, []registry.Candidate{c})
			errorsCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	history, err := a.Revisions(t.Context(), "alpha")
	if err != nil || len(history) != 12 {
		t.Fatalf("conflicting revision allocation: %d, %v", len(history), err)
	}
	ids := make(map[string]bool)
	for i, row := range history {
		if row.Number != i+1 || ids[row.Revision] {
			t.Fatal("duplicate identity or non-monotonic revision")
		}
		ids[row.Revision] = true
	}
	cancel()
	if _, err := a.Apply(ctx, []registry.Candidate{c}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled write = %v", err)
	}
	if _, err := a.Archive(ctx, "alpha"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lifecycle = %v", err)
	}
}

func TestMigrationFailurePreservesState(t *testing.T) {
	for _, mutation := range []string{
		"UPDATE schema_migrations SET checksum='incorrect'",
		"UPDATE schema_migrations SET version=99 WHERE version=1",
	} {
		t.Run(mutation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "db.sqlite")
			s := openTest(t, path)
			if _, err := s.Apply(t.Context(), []registry.Candidate{candidate(t, "alpha", "one")}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.ExecContext(t.Context(), mutation); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(t.Context(), path); err == nil {
				t.Fatal("incompatible schema accepted")
			}
			if _, err := s.Get(t.Context(), "alpha", ""); err != nil {
				t.Fatal("failed migration lost source data", err)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "db.sqlite")
	connector, err := driver.NewConnector(path)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE workflows(existing_data TEXT); INSERT INTO workflows VALUES('preserve')"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("conflicting migration succeeded")
	}
	var value string
	if err := db.QueryRowContext(t.Context(), "SELECT existing_data FROM workflows").Scan(&value); err != nil || value != "preserve" {
		t.Fatalf("migration damaged existing table: %q, %v", value, err)
	}
	var ledgerCount int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master WHERE name='schema_migrations'").Scan(&ledgerCount); err != nil || ledgerCount != 0 {
		t.Fatalf("failed migration partially committed: %d, %v", ledgerCount, err)
	}
}

func TestStorageErrorsAndListing(t *testing.T) {
	t.Parallel()
	s := openTest(t, filepath.Join(t.TempDir(), "space# & dir", "db.sqlite"))
	for _, name := range []string{"zeta", "alpha"} {
		if _, err := s.Apply(t.Context(), []registry.Candidate{candidate(t, name, "one")}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.List(t.Context(), false)
	if err != nil || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Fatalf("listing order = %v, %v", list, err)
	}
	if _, err := s.Revisions(t.Context(), "missing"); !errors.Is(err, registry.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.SetEnabled(t.Context(), "missing", true); !errors.Is(err, registry.ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(t.Context(), false); err == nil {
		t.Fatal("closed database returned success")
	}
	if err := s.Health(t.Context()); err == nil {
		t.Fatal("closed database health succeeded")
	}
	for _, path := range []string{"", "file:invalid", "db?mode=memory"} {
		if _, err := Open(t.Context(), path); err == nil || strings.Contains(err.Error(), path) && path != "" {
			t.Fatal("invalid or unsafe path error", err)
		}
	}
}

func TestInsertFailureRollsBackAndRestore(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	s := openTest(t, path)
	for pragma, want := range map[string]string{"foreign_keys": "1", "journal_mode": "delete", "synchronous": "2", "busy_timeout": "5000"} {
		var value string
		if err := s.db.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&value); err != nil || value != want {
			t.Fatalf("%s = %s, %v", pragma, value, err)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatalf("database permissions are not private: %v", err)
		}
	}
	if _, err := s.db.ExecContext(t.Context(), "CREATE TRIGGER fail_insert BEFORE INSERT ON workflow_revisions BEGIN SELECT RAISE(ABORT,'injected failure'); END"); err != nil {
		t.Fatal(err)
	}
	c := candidate(t, "alpha", "one")
	if _, err := s.Apply(t.Context(), []registry.Candidate{c}); err == nil {
		t.Fatal("failed insert returned success")
	}
	var count int
	if err := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM workflows").Scan(&count); err != nil || count != 0 {
		t.Fatal("failed insert left workflow or pointer behind", err)
	}
	if _, err := s.db.ExecContext(t.Context(), "DROP TRIGGER fail_insert"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(t.Context(), []registry.Candidate{c}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored.sqlite")
	if err := os.WriteFile(restored, data, 0600); err != nil {
		t.Fatal(err)
	}
	copy := openTest(t, restored)
	r, err := copy.Get(t.Context(), "alpha", "")
	if err != nil || !bytes.Equal(r.Source, c.Source) || !bytes.Equal(r.Definition, c.Normalized) {
		t.Fatal("stopped-process backup/restore lost revision", err)
	}
}

func TestCorruptStoredDefinitionIsReported(t *testing.T) {
	t.Parallel()
	s := openTest(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if _, err := s.Apply(t.Context(), []registry.Candidate{candidate(t, "alpha", "one")}); err != nil {
		t.Fatal(err)
	}
	// Simulate out-of-band corruption in this isolated DB, not a supported edit.
	if _, err := s.db.ExecContext(t.Context(), "DROP TRIGGER revisions_no_update; UPDATE workflow_revisions SET normalized='not json'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(t.Context(), "alpha", ""); err == nil {
		t.Fatal("corrupt persisted definition treated as readable")
	}
}

func TestListingLimitIsReportedWithoutPartialResult(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), "db.sqlite"))
	query := `WITH RECURSIVE seq(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM seq WHERE n<10001)
SELECT 'example',printf('%032d',n),n,1,0,'2026-11-08T00:00:00Z','2026-11-08T00:00:00Z','source','{}',1,'{}' FROM seq`
	result, err := s.query(t.Context(), query)
	if !errors.Is(err, registry.ErrResultLimit) || result != nil {
		t.Fatalf("partial or unlimited listing = %d, %v", len(result), err)
	}
}

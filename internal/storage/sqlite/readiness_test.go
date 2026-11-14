package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

func TestReadinessRejectsIncompatibleMigrationChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database with % and #.sqlite")
	s := openTest(t, path)
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.PublishServerState(t.Context(), f, ServerReady); err != nil {
		t.Fatal(err)
	}
	if state, err := ReadServerState(t.Context(), path); err != nil || state != ServerReady {
		t.Fatal(state, err)
	}
	if _, err = s.db.ExecContext(t.Context(), "UPDATE schema_migrations SET checksum='modified' WHERE version=7"); err != nil {
		t.Fatal(err)
	}
	if state, err := ReadServerState(t.Context(), path); state != ServerUnavailable || err == nil {
		t.Fatal(state, err)
	}
	if err = s.EndDispatcher(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	if err = s.PublishServerState(t.Context(), f, ServerReady); !errors.Is(err, execution.ErrConflict) {
		t.Fatal(err)
	}
}

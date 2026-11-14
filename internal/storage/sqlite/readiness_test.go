package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

func BenchmarkServerWorkerStatus(b *testing.B) {
	store, path := benchmarkStore(b)
	b.Cleanup(func() { closeBenchmarkStore(b, store) })
	records, err := store.Apply(b.Context(), benchmarkCandidates(b, 1))
	if err != nil {
		b.Fatal(err)
	}
	cap := execution.DefaultCapacity()
	for range 16 {
		if _, err := store.Enqueue(b.Context(), records[0], execution.QueueAdmission{Capacity: cap}); err != nil {
			b.Fatal(err)
		}
	}
	fence, err := store.BeginDispatcher(b.Context(), testOwner, false)
	if err != nil {
		b.Fatal(err)
	}
	if err := store.PublishServerState(b.Context(), fence, ServerReady, cap.Workers); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		status, err := ReadServerStatus(b.Context(), path)
		if err != nil || status.State != ServerReady || status.Pool == nil || *status.Pool != (WorkerPoolStatus{Capacity: cap.Workers, QueueDepth: 16}) {
			b.Fatal(status, err)
		}
	}
}

func TestReadinessRejectsIncompatibleMigrationChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database with % and #.sqlite")
	s := openTest(t, path)
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.PublishServerState(t.Context(), f, ServerReady, 4); err != nil {
		t.Fatal(err)
	}
	if status, err := ReadServerStatus(t.Context(), path); err != nil || status.State != ServerReady {
		t.Fatal(status, err)
	}
	if _, err = s.db.ExecContext(t.Context(), "UPDATE schema_migrations SET checksum='modified' WHERE version=7"); err != nil {
		t.Fatal(err)
	}
	if status, err := ReadServerStatus(t.Context(), path); status.State != ServerUnavailable || err == nil {
		t.Fatal(status, err)
	}
	if err = s.EndDispatcher(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	if err = s.PublishServerState(t.Context(), f, ServerReady, 4); !errors.Is(err, execution.ErrConflict) {
		t.Fatal(err)
	}
}

func TestWorkerStatusMigrationPreservesQueueAndResetsCapacityOnNewOwner(t *testing.T) {
	s, r, path := reliableStoreFixture(t)
	cap := execution.DefaultCapacity()
	queued, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.PublishServerState(t.Context(), f, ServerReady, cap.Workers); err != nil {
		t.Fatal(err)
	}
	removeMigrationEight(t, s)
	reopened := openTest(t, path)
	if status, err := ReadServerStatus(t.Context(), path); status.State != ServerUnavailable || err == nil {
		t.Fatal(status, err)
	}
	owner, err := opaqueID()
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := reopened.BeginDispatcher(t.Context(), owner, false)
	if err != nil {
		t.Fatal(err)
	}
	if status, err := ReadServerStatus(t.Context(), path); status != (ServerStatus{State: ServerUnavailable}) || err != nil {
		t.Fatal(status, err)
	}
	if err = reopened.PublishServerState(t.Context(), f, ServerReady, cap.Workers); !errors.Is(err, execution.ErrConflict) {
		t.Fatal(err)
	}
	if err = reopened.PublishServerState(t.Context(), fresh, ServerReady, 2); err != nil {
		t.Fatal(err)
	}
	if status, err := ReadServerStatus(t.Context(), path); status.State != ServerReady || status.Pool == nil || *status.Pool != (WorkerPoolStatus{Capacity: 2, QueueDepth: 1}) || err != nil {
		t.Fatal(status, status.Pool, err)
	}
	if got, err := reopened.Execution(t.Context(), queued.Snapshot.ID); got.State != execution.Queued || err != nil {
		t.Fatal(got.State, err)
	}
}

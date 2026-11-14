package sqlite

import (
	"github.com/PHPDevsr/opsweave/internal/dispatchlock"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"os"
	"testing"
)

func BenchmarkDispatcherClaim(b *testing.B) {
	var lock *dispatchlock.Lock
	var store *Store
	var path string
	var record registry.Record
	var fence execution.Fence
	setup := func() {
		if path == "" {
			store, path = benchmarkStore(b)
		} else {
			var err error
			store, err = Open(b.Context(), path)
			if err != nil {
				b.Fatal(err)
			}
		}
		var lockErr error
		lock, lockErr = dispatchlock.Acquire(path)
		if lockErr != nil {
			b.Fatal(lockErr)
		}
		records, err := store.Apply(b.Context(), benchmarkCandidates(b, 1))
		if err != nil {
			b.Fatal(err)
		}
		record = records[0]
		fence, err = store.BeginDispatcher(b.Context(), testOwner, false)
		if err != nil {
			b.Fatal(err)
		}
	}
	setup()
	b.Cleanup(func() {
		closeBenchmarkStore(b, store)
		if err := lock.Close(); err != nil {
			b.Error(err)
		}
	})
	cap := execution.DefaultCapacity()
	count := 0
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		if count == 128 {
			closeBenchmarkStore(b, store)
			if err := lock.Close(); err != nil {
				b.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				b.Fatal(err)
			}
			setup()
			count = 0
		}
		q, err := store.Enqueue(b.Context(), record, execution.QueueAdmission{Capacity: cap})
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		snap, err := store.ClaimExecution(b.Context(), fence, q.Snapshot.ID, record, cap)
		if err != nil || snap.State != execution.Running {
			b.Fatal(snap, err)
		}
		b.StopTimer()
		snap, err = store.RequestReliableCancellation(b.Context(), snap.ID, "")
		if err != nil {
			b.Fatal(err)
		}
		for _, op := range []execution.Operation{execution.CancelIdleSteps, execution.FinishExecution} {
			snap, err = store.ChangeDispatch(b.Context(), fence, execution.ReliableMutation{ID: snap.ID, Owner: fence.Owner, Version: snap.Version, Operation: op})
			if err != nil {
				b.Fatal(err)
			}
		}
		count++
		b.StartTimer()
	}
}

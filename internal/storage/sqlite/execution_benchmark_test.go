package sqlite

import (
	"testing"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

func BenchmarkExecutionHistory(b *testing.B) {
	store, _ := benchmarkStore(b)
	b.Cleanup(func() { closeBenchmarkStore(b, store) })
	records, err := store.Apply(b.Context(), benchmarkCandidates(b, 1))
	if err != nil {
		b.Fatal(err)
	}
	snap, err := store.Submit(b.Context(), records[0], testOwner)
	if err != nil {
		b.Fatal(err)
	}
	for _, m := range []execution.Mutation{{Step: "first", To: execution.Running}, {Step: "first", To: execution.Succeeded}, {To: execution.Succeeded}} {
		m.ID, m.Owner, m.Version = snap.ID, testOwner, snap.Version
		snap, err = store.Change(b.Context(), m)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.Run("Get", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			got, err := store.Execution(b.Context(), snap.ID)
			if err != nil || got.State != execution.Succeeded || len(got.Steps) != 1 {
				b.Fatal(got, err)
			}
		}
	})
	b.Run("Logs", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			events, err := store.Events(b.Context(), snap.ID)
			if err != nil || len(events) != 6 {
				b.Fatal(events, err)
			}
		}
	})
}

package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

func BenchmarkReliableHistory(b *testing.B) {
	store, _ := benchmarkStore(b)
	b.Cleanup(func() { closeBenchmarkStore(b, store) })
	records, err := store.Apply(b.Context(), benchmarkCandidates(b, 1))
	if err != nil {
		b.Fatal(err)
	}
	admission, err := store.CreateReliable(b.Context(), records[0], execution.AdmissionOptions{Owner: testOwner})
	if err != nil {
		b.Fatal(err)
	}
	snap := admission.Snapshot
	for _, m := range []execution.ReliableMutation{{Step: "first", Operation: execution.StartStep}, {Step: "first", Operation: execution.StartAttempt}, {Step: "first", Operation: execution.FinishAttempt, Outcome: execution.Succeeded, Output: json.RawMessage(`{}`)}, {Operation: execution.FinishExecution}} {
		m.ID, m.Owner, m.Version = snap.ID, testOwner, snap.Version
		snap, err = store.ChangeReliable(b.Context(), m)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.Run("Get", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			result, err := store.Execution(b.Context(), snap.ID)
			if err != nil || result.Format != 2 || len(result.Steps[0].Attempts) != 1 {
				b.Fatal(result, err)
			}
		}
	})
	b.Run("Logs", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			events, err := store.Events(b.Context(), snap.ID)
			if err != nil || len(events) != 8 {
				b.Fatal(len(events), err)
			}
		}
	})
}

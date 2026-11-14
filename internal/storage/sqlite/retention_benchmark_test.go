package sqlite

import (
	"testing"
	"time"
)

func BenchmarkExecutionPrune(b *testing.B) {
	store, _ := benchmarkStore(b)
	b.Cleanup(func() { closeBenchmarkStore(b, store) })
	records, err := store.Apply(b.Context(), benchmarkCandidates(b, 1))
	if err != nil {
		b.Fatal(err)
	}
	r := records[0]
	const seed = `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?)
INSERT INTO executions SELECT printf('%032x',x),?,?,'succeeded',0,?,'2020-01-01T00:00:00Z',
'{"updatedAt":"2020-01-01T00:00:00.123456789Z"}' FROM n;`
	b.Run("Apply100", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			b.StopTimer()
			if _, err := store.db.ExecContext(b.Context(), seed, 100, r.Name, r.Revision, testOwner); err != nil {
				b.Fatal(err)
			}
			if _, err := store.db.ExecContext(b.Context(), "INSERT INTO execution_events SELECT id,1,'{}' FROM executions"); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			report, err := store.PruneExecutions(b.Context(), time.Hour, false)
			if err != nil || report.Deleted != 100 {
				b.Fatal(report, err)
			}
		}
	})
	if _, err := store.db.ExecContext(b.Context(), seed, 1000, r.Name, r.Revision, testOwner); err != nil {
		b.Fatal(err)
	}
	b.Run("Preview1000", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			report, err := store.PruneExecutions(b.Context(), time.Hour, true)
			if err != nil || report.Eligible != 1000 || report.Deleted != 0 {
				b.Fatal(report, err)
			}
		}
	})
}

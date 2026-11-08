package sqlite

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/registry"
)

func benchmarkCandidates(b *testing.B, count int) []registry.Candidate {
	b.Helper()
	sources := make([]registry.Source, 0, count)
	for i := range count {
		source := fmt.Sprintf("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: benchmark-%d}\nspec:\n  steps:\n    - id: first\n      uses: test.echo\n      with: {message: hello}\n", i)
		sources = append(sources, registry.Source{Data: []byte(source)})
	}
	catalog := registry.Catalog{"test.echo": {ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"message": {Type: "string", Required: true}}}}}
	candidates, err := registry.Prepare(b.Context(), sources, catalog)
	if err != nil {
		b.Fatal(err)
	}
	return candidates
}

// Every write benchmark reuses one isolated path. Resets close the database before
// removing its test-owned file; immutable production revisions are never pruned.
func resetBenchmarkStore(b *testing.B, store **Store, path string) {
	b.Helper()
	if *store != nil {
		if err := (*store).Close(); err != nil {
			b.Fatal(err)
		}
		*store = nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		b.Fatal(err)
	}
	next, err := Open(b.Context(), path)
	if err != nil {
		b.Fatal(err)
	}
	*store = next
}

func benchmarkStore(b *testing.B) (*Store, string) {
	b.Helper()
	path := filepath.Join(b.TempDir(), "benchmark.sqlite")
	store, err := Open(b.Context(), path)
	if err != nil {
		b.Fatal(err)
	}
	return store, path
}

func closeBenchmarkStore(b *testing.B, store *Store) {
	b.Helper()
	if store != nil {
		if err := store.Close(); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkSQLiteApply(b *testing.B) {
	for _, batch := range []int{1, 16} {
		b.Run(fmt.Sprintf("Batch%d", batch), func(b *testing.B) {
			candidates := benchmarkCandidates(b, batch)
			store, path := benchmarkStore(b)
			b.Cleanup(func() { closeBenchmarkStore(b, store) })
			ctx := b.Context()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Bound each database to 128 revisions. Reset/migration costs are
				// excluded here and measured separately by BenchmarkSQLiteMigrate.
				if i > 0 && i%(128/batch) == 0 {
					b.StopTimer()
					resetBenchmarkStore(b, &store, path)
					b.StartTimer()
				}
				records, err := store.Apply(ctx, candidates)
				if err != nil {
					b.Fatal(err)
				}
				if len(records) != batch {
					b.Fatal("incomplete atomic apply")
				}
				for _, record := range records {
					if record.Number != i%(128/batch)+1 {
						b.Fatal("benchmark revision history was not reset as expected")
					}
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(batch), "revisions/op")
		})
	}
}

func BenchmarkSQLiteGet(b *testing.B) {
	candidates := benchmarkCandidates(b, 1)
	store, _ := benchmarkStore(b)
	b.Cleanup(func() { closeBenchmarkStore(b, store) })
	var first string
	for i := range 128 {
		records, err := store.Apply(b.Context(), candidates)
		if err != nil {
			b.Fatal(err)
		}
		if i == 0 {
			first = records[0].Revision
		}
	}
	for _, test := range []struct{ name, revision string }{{"Current", ""}, {"Historical", first}} {
		b.Run(test.name, func(b *testing.B) {
			ctx := b.Context()
			b.ReportAllocs()
			for b.Loop() {
				record, err := store.Get(ctx, candidates[0].Name, test.revision)
				if err != nil {
					b.Fatal(err)
				}
				if !bytes.Equal(record.Source, candidates[0].Source) {
					b.Fatal("stored definition changed")
				}
			}
		})
	}
}

func BenchmarkSQLiteList(b *testing.B) {
	for _, count := range []int{1, 128} {
		b.Run(fmt.Sprintf("Workflows%d", count), func(b *testing.B) {
			store, _ := benchmarkStore(b)
			b.Cleanup(func() { closeBenchmarkStore(b, store) })
			if _, err := store.Apply(b.Context(), benchmarkCandidates(b, count)); err != nil {
				b.Fatal(err)
			}
			ctx := b.Context()
			b.ReportAllocs()
			for b.Loop() {
				rows, err := store.List(ctx, false)
				if err != nil {
					b.Fatal(err)
				}
				if len(rows) != count {
					b.Fatal("incomplete workflow listing")
				}
			}
			b.ReportMetric(float64(count), "workflows/op")
		})
	}
}

func BenchmarkSQLiteMigrate(b *testing.B) {
	path := filepath.Join(b.TempDir(), "benchmark.sqlite")
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			b.Fatal(err)
		}
		b.StartTimer()
		store, err := Open(ctx, path)
		if err != nil {
			b.Fatal(err)
		}
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

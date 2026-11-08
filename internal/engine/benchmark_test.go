package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

func BenchmarkManualExecution(b *testing.B) {
	ctx := b.Context()
	d := registry.Descriptor{ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"message": {Type: "string", Required: true}}}, Outputs: map[string]registry.Field{}}
	runtime, err := action.New([]action.Registration{{Descriptor: d, Run: func(context.Context, map[string]any) (map[string]any, error) { return map[string]any{}, nil }}}, nil)
	if err != nil {
		b.Fatal(err)
	}
	candidates, err := registry.Prepare(ctx, []registry.Source{{Data: []byte(source)}}, registry.Catalog{d.ID: d})
	if err != nil {
		b.Fatal(err)
	}
	path := filepath.Join(b.TempDir(), "benchmark.sqlite")
	var store *sqlite.Store
	setup := func() {
		var err error
		store, err = sqlite.Open(ctx, path)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := store.Apply(ctx, candidates); err != nil {
			b.Fatal(err)
		}
	}
	setup()
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Error(err)
		}
	})
	e := Engine{Workflows: store, Executions: store, Actions: runtime}
	count := 0
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if count == 128 {
			b.StopTimer()
			if err := store.Close(); err != nil {
				b.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				b.Fatal(err)
			}
			setup()
			e.Workflows, e.Executions = store, store
			count = 0
			b.StartTimer()
		}
		snap, err := e.Run(ctx, "example")
		if err != nil || snap.State != execution.Succeeded || len(snap.Steps) != 2 {
			b.Fatal(snap, err)
		}
		count++
	}
}

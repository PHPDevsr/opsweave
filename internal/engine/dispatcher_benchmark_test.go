package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/dispatchlock"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

// This measures the complete bounded dispatch path with no-op test handlers,
// not external-action parallel speedup. Ownership setup is outside the timer.
func BenchmarkDispatcherThroughput(b *testing.B) {
	for _, workers := range []int{1, 4} {
		b.Run(fmt.Sprintf("Workers%d", workers), func(b *testing.B) {
			descriptor := registry.Descriptor{ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object"}, Outputs: map[string]registry.Field{}}
			runtime, err := action.New([]action.Registration{{Descriptor: descriptor, Run: func(context.Context, map[string]any) (map[string]any, error) { return map[string]any{}, nil }}}, nil)
			if err != nil {
				b.Fatal(err)
			}
			source := "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  parallelism: 4\n  steps:\n"
			for i := range 8 {
				source += fmt.Sprintf("    - {id: step-%d, uses: test.echo, needs: []}\n", i)
			}
			candidates, err := registry.PrepareDispatcher(b.Context(), []registry.Source{{Data: []byte(source)}}, registry.Catalog{descriptor.ID: descriptor})
			if err != nil {
				b.Fatal(err)
			}
			path := filepath.Join(b.TempDir(), "benchmark.sqlite")
			var lock *dispatchlock.Lock
			var store *sqlite.Store
			var d Dispatcher
			setup := func() {
				var err error
				store, err = sqlite.Open(b.Context(), path)
				if err != nil {
					b.Fatal(err)
				}
				lock, err = dispatchlock.Acquire(path)
				if err != nil {
					b.Fatal(err)
				}
				if _, err = store.Apply(b.Context(), candidates); err != nil {
					b.Fatal(err)
				}
				fence, err := store.BeginDispatcher(b.Context(), strings.Repeat("a", 32), false)
				if err != nil {
					b.Fatal(err)
				}
				cap := execution.DefaultCapacity()
				cap.Workers = workers
				d = Dispatcher{Engine: Engine{Workflows: store, Executions: store, Actions: runtime}, Repository: store, Fence: fence, Capacity: cap}
			}
			setup()
			b.Cleanup(func() {
				if err := store.Close(); err != nil {
					b.Error(err)
				}
				if err := lock.Close(); err != nil {
					b.Error(err)
				}
			})
			count := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if count == 128 {
					b.StopTimer()
					if err := store.Close(); err != nil {
						b.Fatal(err)
					}
					if err := lock.Close(); err != nil {
						b.Fatal(err)
					}
					if err := os.Remove(path); err != nil {
						b.Fatal(err)
					}
					setup()
					count = 0
					b.StartTimer()
				}
				q, err := d.Engine.Enqueue(b.Context(), store, d.Capacity, "example", RunOptions{})
				if err != nil {
					b.Fatal(err)
				}
				snap, err := d.Run(b.Context(), q.Snapshot.ID)
				if err != nil || snap.State != execution.Succeeded || !snap.Managed {
					b.Fatal(snap, err)
				}
				count++
			}
		})
	}
}

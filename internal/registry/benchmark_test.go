package registry

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkPrepare(b *testing.B) {
	for _, test := range []struct {
		name         string
		steps, batch int
	}{
		{"Steps1", 1, 1},
		{"Steps32", 32, 1},
		{"Steps256", 256, 1},
		{"Batch16", 1, 16},
	} {
		b.Run(test.name, func(b *testing.B) {
			sources := make([]Source, 0, test.batch)
			total := 0
			for n := range test.batch {
				var source strings.Builder
				fmt.Fprintf(&source, "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: benchmark-%d}\nspec:\n  steps:\n", n)
				for i := range test.steps {
					fmt.Fprintf(&source, "    - id: step-%d\n      uses: test.echo\n      with: {message: hello}\n", i)
				}
				data := []byte(source.String())
				sources = append(sources, Source{data})
				total += len(data)
			}
			catalog := fixtureCatalog()
			ctx := b.Context()
			b.ReportAllocs()
			b.SetBytes(int64(total))
			for b.Loop() {
				result, err := Prepare(ctx, sources, catalog)
				if err != nil {
					b.Fatal(err)
				}
				if len(result) != test.batch {
					b.Fatal("incomplete preflight batch")
				}
			}
			b.ReportMetric(float64(test.batch), "workflows/op")
		})
	}
}

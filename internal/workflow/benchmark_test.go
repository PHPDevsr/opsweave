package workflow

import (
	"fmt"
	"strings"
	"testing"
)

func benchmarkDefinition(steps int) []byte {
	var source strings.Builder
	source.WriteString("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: benchmark}\nspec:\n  steps:\n")
	for i := range steps {
		fmt.Fprintf(&source, "    - id: step-%d\n      uses: test.echo\n      with: {message: hello}\n", i)
	}
	return []byte(source.String())
}

func BenchmarkParse(b *testing.B) {
	for _, count := range []int{1, 32, 256} {
		b.Run(fmt.Sprintf("Steps%d", count), func(b *testing.B) {
			source := benchmarkDefinition(count)
			ctx := b.Context()
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				d, err := Parse(ctx, source)
				if err != nil {
					b.Fatal(err)
				}
				if len(d.Order) != count {
					b.Fatal("incomplete dependency plan")
				}
			}
		})
	}
}

func BenchmarkValidateCondition(b *testing.B) {
	for _, test := range []struct{ name, source string }{
		{"Literal", "true"},
		{"StepPredicates", "steps.first.success && !steps.second.failed"},
		{"Terms128", strings.Repeat("true && ", 127) + "true"},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(test.source)))
			for b.Loop() {
				if _, err := ValidateCondition(test.source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

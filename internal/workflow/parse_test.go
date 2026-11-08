package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const minimal = `apiVersion: opsweave.io/v1
kind: Workflow
metadata:
  name: sample
spec:
  steps:
    - id: first
      uses: test.echo
`

func parseOK(t *testing.T, source string) *Definition {
	t.Helper()
	d, err := Parse(t.Context(), []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestParseDefaultsAndSource(t *testing.T) {
	t.Parallel()
	source := []byte("\xef\xbb\xbf---\n# preserve this\n" + minimal)
	d, err := Parse(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if d.APIVersion != "opsweave.io/v1" || d.Kind != "Workflow" || d.Metadata.Name != "sample" || len(d.Spec.Steps) != 1 {
		t.Fatalf("unexpected definition: %+v", d)
	}
	if d.Spec.Timeout != time.Hour || d.Spec.Concurrency != 1 || d.Spec.Parallelism != 1 {
		t.Fatalf("unexpected defaults: %+v", d.Spec)
	}
	s := d.Spec.Steps[0]
	if s.Timeout != 5*time.Minute || s.Retry != (Retry{1, time.Second, time.Minute}) || len(s.Inputs) != 0 {
		t.Fatalf("unexpected step defaults: %+v", s)
	}
	if !bytes.Equal(source, d.Source()) {
		t.Fatal("source was not preserved")
	}
	copy := d.Source()
	copy[0] = 'x'
	source[0] = 'y'
	if d.Source()[0] != 0xef {
		t.Fatal("source aliases caller memory")
	}
	if err := d.Phase01(); err != nil {
		t.Fatal(err)
	}
}

func TestParseLiteralsAndMetadata(t *testing.T) {
	source := strings.Replace(minimal, "  name: sample", "  name: sample\n  description: Example\n  labels: {team: ops}", 1) + `      with:
        message: ${literal}
        date: 2026-11-08
        nested: {values: [true, 42, -4, 1.25, "42"]}
`
	d := parseOK(t, source)
	want := map[string]any{"message": "${literal}", "date": "2026-11-08", "nested": map[string]any{"values": []any{true, int64(42), int64(-4), 1.25, "42"}}}
	if !reflect.DeepEqual(d.Spec.Steps[0].Inputs, want) || d.Metadata.Description != "Example" || d.Metadata.Labels["team"] != "ops" {
		t.Fatalf("unexpected result: %+v", d)
	}
}

func TestParseRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct{ name, source, code string }{
		{"empty", "", "invalid_yaml"},
		{"syntax", "[unterminated", "invalid_yaml"},
		{"root array", "[]", "expected_mapping"},
		{"multiple", minimal + "---\n{}", "multiple_documents"},
		{"trailing broken document", minimal + "---\n[", "multiple_documents"},
		{"missing version", strings.Replace(minimal, "apiVersion: opsweave.io/v1\n", "", 1), "expected_string"},
		{"version type", strings.Replace(minimal, "opsweave.io/v1", "42", 1), "expected_string"},
		{"version", strings.Replace(minimal, "opsweave.io/v1", "opsweave.io/v2", 1), "unsupported_version"},
		{"kind missing", strings.Replace(minimal, "kind: Workflow\n", "", 1), "expected_string"},
		{"kind", strings.Replace(minimal, "kind: Workflow", "kind: Job", 1), "invalid_kind"},
		{"unknown root", minimal + "unknown: sensitive-marker\n", "unknown_field"},
		{"duplicate key", minimal + "kind: Workflow\n", "invalid_mapping_key"},
		{"nonstring key", minimal + "42: value\n", "invalid_mapping_key"},
		{"compound key", minimal + "? [a, b]\n: value\n", "invalid_mapping_key"},
		{"merge key", minimal + "<<: {}\n", "invalid_mapping_key"},
		{"anchor", strings.Replace(minimal, "metadata:", "metadata: &meta", 1), "unsupported_yaml"},
		{"alias", minimal + "alias: *missing\n", "invalid_yaml"},
		{"explicit tag", strings.Replace(minimal, "name: sample", "name: !!str sample", 1), "unsupported_yaml"},
		{"custom tag", strings.Replace(minimal, "name: sample", "name: !custom sample", 1), "unsupported_yaml"},
		{"null", strings.Replace(minimal, "name: sample", "name: null", 1), "invalid_scalar"},
		{"metadata missing", strings.Replace(minimal, "metadata:\n  name: sample\n", "", 1), "expected_mapping"},
		{"unknown metadata", strings.Replace(minimal, "name: sample", "name: sample\n  other: value", 1), "unknown_field"},
		{"name type", strings.Replace(minimal, "name: sample", "name: true", 1), "expected_string"},
		{"name", strings.Replace(minimal, "name: sample", "name: UPPER", 1), "invalid_identifier"},
		{"description type", strings.Replace(minimal, "name: sample", "name: sample\n  description: []", 1), "expected_string"},
		{"description size", strings.Replace(minimal, "name: sample", "name: sample\n  description: "+strings.Repeat("x", 4097), 1), "resource_limit"},
		{"labels type", strings.Replace(minimal, "name: sample", "name: sample\n  labels: []", 1), "invalid_labels"},
		{"label key", strings.Replace(minimal, "name: sample", "name: sample\n  labels: {UPPER: x}", 1), "invalid_identifier"},
		{"label value type", strings.Replace(minimal, "name: sample", "name: sample\n  labels: {team: 1}", 1), "expected_string"},
		{"label value size", strings.Replace(minimal, "name: sample", "name: sample\n  labels: {team: "+strings.Repeat("x", 257)+"}", 1), "resource_limit"},
		{"spec missing", "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: sample}", "expected_mapping"},
		{"unknown spec", strings.Replace(minimal, "spec:", "spec:\n  mystery: true", 1), "unknown_field"},
		{"triggers", strings.Replace(minimal, "spec:", "spec:\n  triggers: []", 1), "unsupported_capability"},
		{"concurrency type", strings.Replace(minimal, "spec:", "spec:\n  concurrency: 1", 1), "expected_mapping"},
		{"concurrency unknown", strings.Replace(minimal, "spec:", "spec:\n  concurrency: {other: 1}", 1), "unknown_field"},
		{"concurrency range", strings.Replace(minimal, "spec:", "spec:\n  concurrency: {limit: 0}", 1), "invalid_integer"},
		{"parallelism type", strings.Replace(minimal, "spec:", "spec:\n  parallelism: '1'", 1), "invalid_integer"},
		{"parallelism range", strings.Replace(minimal, "spec:", "spec:\n  parallelism: 257", 1), "invalid_integer"},
		{"steps empty", strings.Replace(minimal, "steps:\n    - id: first\n      uses: test.echo", "steps: []", 1), "invalid_steps"},
		{"steps missing", "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: sample}\nspec: {}", "invalid_steps"},
		{"step type", strings.Replace(minimal, "- id: first\n      uses: test.echo", "- value", 1), "expected_mapping"},
		{"step unknown", minimal + "      other: true\n", "unknown_field"},
		{"step id", strings.Replace(minimal, "id: first", "id: bad--id", 1), "invalid_identifier"},
		{"uses missing", strings.Replace(minimal, "      uses: test.echo\n", "", 1), "expected_string"},
		{"uses bad", strings.Replace(minimal, "test.echo", "bad", 1), "invalid_action"},
		{"uses segment", strings.Replace(minimal, "test.echo", "test.bad__thing", 1), "invalid_action"},
		{"input type", minimal + "      with: []\n", "expected_mapping"},
		{"input null", minimal + "      with: {value: null}\n", "invalid_scalar"},
		{"input infinity", minimal + "      with: {value: .inf}\n", "invalid_number"},
		{"input nan", minimal + "      with: {value: .nan}\n", "invalid_number"},
		{"input integer overflow", minimal + "      with: {value: 9223372036854775808}\n", "invalid_number"},
		{"condition type", minimal + "      if: true\n", "expected_string"},
		{"condition empty", minimal + "      if: ''\n", "invalid_condition"},
		{"condition invalid", minimal + "      if: 'true == false'\n", "invalid_condition"},
		{"dependency type", minimal + "      needs: first\n", "invalid_dependencies"},
		{"dependency invalid id", minimal + "      needs: [BAD]\n", "invalid_identifier"},
		{"dependency duplicate", minimal + "      needs: [other, other]\n", "duplicate_dependency"},
		{"dependency missing", minimal + "      needs: [other]\n", "invalid_dependency"},
		{"dependency self", minimal + "      needs: [first]\n", "invalid_dependency"},
		{"duplicate step", minimal + "    - id: first\n      uses: test.echo\n", "duplicate_step"},
		{"cycle", minimal + "      needs: [second]\n    - id: second\n      uses: test.echo\n", "dependency_cycle"},
		{"condition scope", minimal + "      if: steps.missing.success\n", "reference_scope"},
		{"output scope", minimal + "      with: {value: {$ref: steps.first.outputs.body}}\n", "reference_scope"},
		{"reference malformed", minimal + "      with: {value: {$ref: steps.first.outputs.body.nested}}\n", "invalid_reference"},
		{"reference extra field", minimal + "      with: {value: {$ref: secrets.prod.name, extra: true}}\n", "invalid_reference"},
		{"reference type", minimal + "      with: {value: {$ref: 42}}\n", "expected_string"},
		{"reference root", minimal + "      with: {$ref: secrets.prod.name}\n", "expected_mapping"},
		{"retry type", minimal + "      retry: []\n", "expected_mapping"},
		{"retry unknown", minimal + "      retry: {other: 1}\n", "unknown_field"},
		{"retry attempts", minimal + "      retry: {attempts: 11}\n", "invalid_integer"},
		{"retry backoff", minimal + "      retry: {backoff: 0s}\n", "invalid_duration"},
		{"retry max", minimal + "      retry: {maxBackoff: 25h}\n", "invalid_duration"},
		{"retry ordering", minimal + "      retry: {backoff: 2m}\n", "invalid_backoff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(t.Context(), []byte(tt.source))
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Code != tt.code {
				t.Fatalf("error = %v, want %s", err, tt.code)
			}
			if strings.Contains(err.Error(), "sensitive-marker") || len(err.Error()) > 200 {
				t.Fatalf("unsafe diagnostic: %v", err)
			}
		})
	}
}

func TestDurationValidation(t *testing.T) {
	for _, value := range []string{"0s", "-1s", "+1s", "1.5s", "1h2m", "1d", "25h", "", "999999999999999999999h", "1 s", "1S", "ms"} {
		t.Run(value, func(t *testing.T) {
			_, err := Parse(t.Context(), []byte(minimal+"      timeout: '"+value+"'\n"))
			if err == nil {
				t.Fatal("invalid duration accepted")
			}
		})
	}
	for _, value := range []string{"1ms", "1s", "1m", "24h"} {
		parseOK(t, minimal+"      timeout: "+value+"\n")
	}
	for _, extra := range []string{"      timeout: 42\n", "      retry: {attempts: 0}\n", "      retry: {attempts: true}\n"} {
		if _, err := Parse(t.Context(), []byte(minimal+extra)); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	for _, source := range [][]byte{bytes.Repeat([]byte("x"), MaxSourceBytes+1), {0xff}} {
		if _, err := Parse(t.Context(), source); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Parse(ctx, []byte(minimal)); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	for _, n := range []int{257, 300} {
		var steps strings.Builder
		steps.WriteString(strings.Split(minimal, "  steps:")[0] + "  steps:\n")
		for i := range n {
			fmt.Fprintf(&steps, "    - id: step-%d\n      uses: test.echo\n", i)
		}
		if _, err := Parse(t.Context(), []byte(steps.String())); err == nil {
			t.Fatal("step limit not enforced")
		}
	}
	for _, input := range []string{
		strings.Repeat("[", 40) + "x" + strings.Repeat("]", 40),
		"[" + strings.Repeat("x,", 20000) + "x]",
	} {
		if _, err := Parse(t.Context(), []byte(minimal+"      with: {value: "+input+"}\n")); err == nil {
			t.Fatal("input resource limit not enforced")
		}
	}
}

func TestGraphAndReferences(t *testing.T) {
	source := minimal + `    - id: last
      uses: test.echo
      needs: [middle]
      if: steps.first.success && !steps.middle.failed
      with:
        values:
          - $ref: steps.first.outputs.body
          - nested: {$ref: secrets.prod.token}
    - id: middle
      uses: test.echo
      needs: [first]
`
	d := parseOK(t, source)
	if !reflect.DeepEqual(d.Order, []string{"first", "middle", "last"}) {
		t.Fatalf("order = %v", d.Order)
	}
	values := d.Spec.Steps[1].Inputs["values"].([]any)
	if values[0] != (Reference{Kind: "output", Step: "first", Output: "body"}) || values[1].(map[string]any)["nested"] != (Reference{Kind: "secret", Environment: "prod", Name: "token"}) {
		t.Fatalf("references = %#v", values)
	}
	if d.Phase01() == nil {
		t.Fatal("future capabilities accepted")
	}
	if _, err := Parse(t.Context(), []byte(strings.Replace(source, "steps.first.outputs.body", "steps.last.outputs.body", 1))); err == nil {
		t.Fatal("self output reference accepted")
	}
	sequential := parseOK(t, minimal+"    - id: second\n      uses: test.echo\n")
	if !reflect.DeepEqual(sequential.Spec.Steps[1].Needs, []string{"first"}) || sequential.Phase01() != nil {
		t.Fatal("sequential default not preserved")
	}
	for _, extra := range []string{"      if: 'true'\n", "      timeout: 1s\n", "      retry: {attempts: 2}\n", "      with: {value: [{$ref: secrets.prod.token}]}\n", "    - id: root\n      uses: test.echo\n      needs: []\n"} {
		if parseOK(t, minimal+extra).Phase01() == nil {
			t.Fatalf("future capability accepted: %s", extra)
		}
	}
	for _, extra := range []string{"timeout: 1s", "parallelism: 2", "concurrency: {limit: 2}"} {
		d := parseOK(t, strings.Replace(minimal, "spec:", "spec:\n  "+extra, 1))
		if d.Phase01() == nil {
			t.Fatal("future spec policy accepted")
		}
	}
}

func TestIdentifier(t *testing.T) {
	parseOK(t, strings.Replace(minimal, "test.echo", "http.health_check", 1))
	for _, value := range []string{"a", "abc-123", strings.Repeat("a", 63)} {
		if !identifier(value) {
			t.Errorf("valid id rejected: %q", value)
		}
	}
	for _, value := range []string{"", "1a", "a-", "a--b", "a_b", "a.b", "é", strings.Repeat("a", 64)} {
		if identifier(value) {
			t.Errorf("invalid id accepted: %q", value)
		}
	}
}

func TestSafeErrors(t *testing.T) {
	for _, source := range []string{
		minimal + "      with: {sensitive-marker: {$ref: sensitive-marker}}\n",
		minimal + "      sensitive-marker: value\n",
		minimal + "      with: {value: [sensitive-marker\n",
	} {
		_, err := Parse(t.Context(), []byte(source))
		if err == nil || strings.Contains(err.Error(), "sensitive-marker") {
			t.Fatalf("unsafe error = %v", err)
		}
	}
}

func TestCollectionBoundaries(t *testing.T) {
	var steps strings.Builder
	steps.WriteString(strings.Split(minimal, "  steps:")[0] + "  steps:\n")
	for i := range 256 {
		fmt.Fprintf(&steps, "    - id: step-%d\n      uses: test.echo\n", i)
	}
	if len(parseOK(t, steps.String()).Spec.Steps) != 256 {
		t.Fatal("maximum step count rejected")
	}
	var labels strings.Builder
	for i := range 256 {
		fmt.Fprintf(&labels, "    label-%d: value\n", i)
	}
	withLabels := strings.Replace(minimal, "  name: sample", "  name: sample\n  labels:\n"+strings.TrimSuffix(labels.String(), "\n"), 1)
	if len(parseOK(t, withLabels).Metadata.Labels) != 256 {
		t.Fatal("maximum label count rejected")
	}
	if _, err := Parse(t.Context(), []byte(strings.Replace(withLabels, "spec:", "    extra: value\nspec:", 1))); err == nil {
		t.Fatal("excess labels accepted")
	}
	deps := make([]string, 65)
	for i := range deps {
		deps[i] = fmt.Sprintf("step-%d", i)
	}
	if _, err := Parse(t.Context(), []byte(minimal+"      needs: ["+strings.Join(deps, ",")+"]\n")); err == nil {
		t.Fatal("excess dependencies accepted")
	}
	maximum := minimal + "#" + strings.Repeat("x", MaxSourceBytes-len(minimal)-1)
	parseOK(t, maximum)
	parseOK(t, strings.Replace(minimal, "spec:", "spec:\n  concurrency: {limit: 256}\n  parallelism: 256\n  timeout: 24h", 1))
	parseOK(t, minimal+"      retry: {attempts: 10, backoff: 1ms, maxBackoff: 24h}\n")
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{minimal, "[]", "a: &a [*a]", minimal + "      with: {x: {$ref: steps.first.outputs.x}}\n"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		d, err := Parse(ctx, source)
		if err != nil {
			if len(err.Error()) > 200 {
				t.Fatal("unbounded diagnostic")
			}
			return
		}
		if len(d.Spec.Steps) < 1 || len(d.Spec.Steps) > 256 || len(d.Order) != len(d.Spec.Steps) || !bytes.Equal(d.Source(), source) {
			t.Fatal("invalid parse result")
		}
	})
}

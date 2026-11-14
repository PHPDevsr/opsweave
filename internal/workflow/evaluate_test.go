package workflow

import (
	"strings"
	"testing"
)

func TestEvaluateCondition(t *testing.T) {
	outcomes := map[string]string{"a": "succeeded", "b": "failed", "c": "cancelled", "d": "skipped", "e": "timed_out", "busy": "running"}
	for _, tc := range []struct {
		source  string
		want    bool
		invalid bool
	}{
		{"true", true, false}, {"false", false, false}, {"!false && true", true, false},
		{"false || true && !true", false, false}, {"(false || true) && true", true, false},
		{"steps.a.success && steps.b.failed", true, false}, {"steps.a.failed", false, false},
		{"steps.c.cancelled && steps.d.skipped && steps.e.timed_out", true, false},
		{"true || steps.missing.success", true, false}, {"false && steps.busy.failed", false, false},
		{"(true || steps.missing.success) && steps.a.success", true, false},
		{"!(false && steps.missing.success)", true, false},
		{"steps.missing.success", false, true}, {"steps.busy.failed", false, true},
		{"true || broken", false, true}, {"false && (", false, true},
	} {
		t.Run(tc.source, func(t *testing.T) {
			got, err := EvaluateCondition(tc.source, outcomes)
			if (err != nil) != tc.invalid || (!tc.invalid && got != tc.want) {
				t.Fatal(got, err)
			}
		})
	}
}

func FuzzEvaluateCondition(f *testing.F) {
	for _, s := range []string{"true", "false && steps.x.success", "steps.x.success", strings.Repeat("!", 33) + "true"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			return
		}
		_, validation := ValidateCondition(s)
		_, err := EvaluateCondition(s, map[string]string{"x": "succeeded"})
		if validation != nil && err == nil {
			t.Fatal("evaluation bypassed syntax validation")
		}
	})
}

func BenchmarkEvaluateCondition(b *testing.B) {
	outcomes := map[string]string{"a": "succeeded", "b": "failed"}
	for b.Loop() {
		ok, err := EvaluateCondition("steps.a.success && (steps.b.failed || false)", outcomes)
		if err != nil || !ok {
			b.Fatal(ok, err)
		}
	}
}

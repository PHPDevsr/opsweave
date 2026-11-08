package workflow

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidateCondition(t *testing.T) {
	for _, source := range []string{"true", "false", "!false", "true || false && !true", "(true || false) && true", " \t\nsteps.a.success && !steps.b.failed", "steps.a.skipped || steps.b.cancelled || steps.c.timed_out"} {
		if _, err := ValidateCondition(source); err != nil {
			t.Errorf("valid condition %q: %v", source, err)
		}
	}
	refs, err := ValidateCondition("steps.a.success || (steps.b.failed && steps.a.skipped)")
	if err != nil || !reflect.DeepEqual(refs, []string{"a", "b", "a"}) {
		t.Fatalf("references = %v, err = %v", refs, err)
	}
	for _, source := range []string{"", "truefalse", "true ||", "true &&", "true false", "(true", "true)", "True", "steps.a.running", "steps..success", "steps.a- .success", "steps.1.success", "steps.a.success.extra", "steps.a.success()", "true\r", "${true}", "true == true", strings.Repeat("!", 33) + "true", strings.Repeat("(", 33) + "true" + strings.Repeat(")", 33), strings.Repeat("true||", 600) + "true", strings.Repeat("x", 4097)} {
		if _, err := ValidateCondition(source); err == nil {
			t.Errorf("invalid condition accepted: %q", source)
		}
	}
	if _, err := ValidateCondition(strings.Repeat("!", 32) + "true"); err != nil {
		t.Fatal(err)
	}
}

func FuzzValidateCondition(f *testing.F) {
	for _, seed := range []string{"true", "steps.first.success && !false", "", "(((("} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		refs, err := ValidateCondition(source)
		if err != nil && len(err.Error()) > 200 {
			t.Fatal("unbounded diagnostic")
		}
		for _, ref := range refs {
			if !identifier(ref) {
				t.Fatal("invalid reference returned")
			}
		}
	})
}

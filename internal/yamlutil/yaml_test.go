package yamlutil

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStrictLoad(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"a: value", "[true, 42, -1, 1.5]", "date: 2026-11-08", "\xef\xbb\xbf---\na: value"} {
		if _, err := Load(t.Context(), []byte(source), 1024); err != nil {
			t.Fatalf("valid source rejected: %v", err)
		}
	}
	for _, source := range []string{"", "[secret-marker", "a: &a value", "a: !!str value", "a: null", "a: .nan", "a: .inf", "a: 9223372036854775808", "1: value", "a: one\na: two", "<<: {}", "a: value\n---\nb: other", "a: *missing", string([]byte{0xff}), strings.Repeat("[", 40) + "x" + strings.Repeat("]", 40), "[" + strings.Repeat("x,", 20000) + "x]"} {
		_, err := Load(t.Context(), []byte(source), 100000)
		var validation *Error
		if !errors.As(err, &validation) || strings.Contains(err.Error(), "secret-marker") || len(err.Error()) > 100 {
			t.Fatalf("unsafe or missing error: %v", err)
		}
	}
	if _, err := Load(t.Context(), []byte("a: value"), 3); err == nil {
		t.Fatal("source size limit not enforced")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Load(ctx, []byte("a: value"), 1024); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

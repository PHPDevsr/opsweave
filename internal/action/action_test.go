package action

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
)

func descriptor() registry.Descriptor {
	return registry.Descriptor{ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"message": {Type: "string", Required: true}}}, Outputs: map[string]registry.Field{"value": {Type: "string", Required: true}, "secret": {Type: "string", Sensitive: true}, "nested": {Type: "array", Items: &registry.Field{Type: "object", Properties: map[string]registry.Field{"private": {Type: "string", Sensitive: true}, "ok": {Type: "boolean"}}}}}}
}
func TestRuntimeCaptureAndOwnership(t *testing.T) {
	d := descriptor()
	r, err := New([]Registration{{d, func(ctx context.Context, in map[string]any) (map[string]any, error) {
		in["message"] = "changed"
		return map[string]any{"value": "ok", "secret": "top-secret", "nested": []any{map[string]any{"private": "also-secret", "ok": true}}}, nil
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	d.Outputs["value"] = registry.Field{Type: "boolean"}
	c, err := r.Catalog(map[string]string{"test.echo": "1"})
	if err != nil {
		t.Fatal(err)
	}
	c["test.echo"].Outputs["value"] = registry.Field{Type: "integer"}
	in := map[string]any{"message": "original"}
	b, err := r.Invoke(t.Context(), "test.echo", "1", in)
	if err != nil || string(b) != `{"nested":[{"ok":true}],"value":"ok"}` || in["message"] != "original" {
		t.Fatalf("%s %v %v", b, in, err)
	}
}

func TestRuntimeRejectsInvalidAndUnsafeCapture(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  map[string]any
		want error
	}{
		{"nil", nil, execution.ErrOutput}, {"missing", map[string]any{}, execution.ErrOutput},
		{"unknown", map[string]any{"value": "ok", "unknown": "secret"}, execution.ErrOutput},
		{"wrong type", map[string]any{"value": int64(1)}, execution.ErrOutput},
		{"invalid UTF-8", map[string]any{"value": string([]byte{0xff})}, execution.ErrOutput},
		{"large", map[string]any{"value": strings.Repeat("x", 65537)}, execution.ErrOutput},
		{"escaped size", map[string]any{"value": strings.Repeat("\x00", 12000)}, execution.ErrOutput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := New([]Registration{{descriptor(), func(context.Context, map[string]any) (map[string]any, error) { return tc.out, nil }}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = r.Invoke(t.Context(), "test.echo", "1", map[string]any{"message": "hello"})
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
		})
	}
	r, err := New([]Registration{{descriptor(), func(context.Context, map[string]any) (map[string]any, error) {
		return nil, errors.New("credential=do-not-log")
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Invoke(t.Context(), "test.echo", "1", map[string]any{"message": "ok"})
	if err != execution.ErrAction || strings.Contains(err.Error(), "credential") {
		t.Fatal(err)
	}
	_, err = r.Invoke(t.Context(), "test.echo", "1", map[string]any{"message": true})
	if err != execution.ErrInvalid {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.Invoke(ctx, "test.echo", "1", nil); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := r.Invoke(t.Context(), "test.echo", "other", nil); err != execution.ErrUnavailable {
		t.Fatal(err)
	}
	var absent *Runtime
	if absent.Check("test.echo", "1") != execution.ErrUnavailable {
		t.Fatal("nil runtime")
	}
}

func TestRuntimeRegistrationAndPermissions(t *testing.T) {
	handler := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{"value": "ok"}, nil
	}
	d := descriptor()
	d.Permissions = []string{"network"}
	r, err := New([]Registration{{d, handler}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Check(d.ID, d.Version) != execution.ErrPermission {
		t.Fatal("missing grant")
	}
	if _, err := r.Catalog(map[string]string{d.ID: d.Version}); err != execution.ErrPermission {
		t.Fatal(err)
	}
	r, err = New([]Registration{{d, handler}}, []string{"network"})
	if err != nil || r.Check(d.ID, d.Version) != nil {
		t.Fatal(err)
	}
	for _, grant := range []string{"", "*"} {
		if _, err := New(nil, []string{grant}); err != execution.ErrPermission {
			t.Fatal(err)
		}
	}
	for _, edit := range []func(*registry.Descriptor){func(d *registry.Descriptor) { d.Version = "" }, func(d *registry.Descriptor) { d.APIVersion = "future" }, func(d *registry.Descriptor) { d.Inputs.Type = "unknown" }, func(d *registry.Descriptor) { d.Outputs["bad"] = registry.Field{Type: "unknown"} }, func(d *registry.Descriptor) { d.Permissions = []string{"*"} }, func(d *registry.Descriptor) { d.Capabilities = []string{"unknown"} }} {
		d := descriptor()
		edit(&d)
		if _, err := New([]Registration{{d, handler}}, nil); err == nil {
			t.Fatal("invalid descriptor")
		}
	}
	if _, err := New([]Registration{{descriptor(), nil}}, nil); err == nil {
		t.Fatal("nil handler")
	}
	if _, err := New([]Registration{{descriptor(), handler}, {descriptor(), handler}}, nil); err == nil {
		t.Fatal("duplicate version")
	}
	cyclic := registry.Field{Type: "array"}
	cyclic.Items = &cyclic
	d = descriptor()
	d.Outputs["cycle"] = cyclic
	if _, err := New([]Registration{{d, handler}}, nil); err == nil {
		t.Fatal("cycle")
	}
}

func TestCaptureScalarAndCollectionBounds(t *testing.T) {
	credentialBudget := captureBudget{}
	if _, ok := capture("inline", registry.Field{Type: "string", Credential: true}, 0, &credentialBudget, false); ok {
		t.Fatal("inline credential accepted")
	}
	cases := []struct {
		value any
		field registry.Field
		valid bool
	}{
		{true, registry.Field{Type: "boolean"}, true}, {"true", registry.Field{Type: "boolean"}, false},
		{int64(3), registry.Field{Type: "integer"}, true}, {3, registry.Field{Type: "integer"}, false},
		{int64(3), registry.Field{Type: "number"}, true}, {1.5, registry.Field{Type: "number"}, true},
		{math.NaN(), registry.Field{Type: "number"}, false}, {math.Inf(1), registry.Field{Type: "number"}, false}, {true, registry.Field{Type: "number"}, false},
		{[]any{"secret"}, registry.Field{Type: "array", Items: &registry.Field{Type: "string", Sensitive: true}}, true},
		{[]any{true}, registry.Field{Type: "array", Items: &registry.Field{Type: "string"}}, false},
		{"bad", registry.Field{Type: "array", Items: &registry.Field{Type: "string"}}, false},
		{[]any{}, registry.Field{Type: "array"}, false}, {nil, registry.Field{Type: "unknown"}, false},
	}
	for _, tc := range cases {
		b := captureBudget{}
		_, ok := capture(tc.value, tc.field, 0, &b, true)
		if ok != tc.valid {
			t.Fatalf("%v %#v", tc.value, tc.field)
		}
	}
	for _, b := range []captureBudget{{nodes: 20000}, {bytes: 65537}} {
		if _, ok := capture("ok", registry.Field{Type: "string"}, 0, &b, false); ok {
			t.Fatal("budget")
		}
	}
	b := captureBudget{}
	if _, ok := capture("ok", registry.Field{Type: "string"}, 33, &b, false); ok {
		t.Fatal("depth")
	}
	b = captureBudget{}
	v, ok := capture([]any{"secret"}, registry.Field{Type: "array", Items: &registry.Field{Type: "string", Sensitive: true}}, 0, &b, true)
	data, err := json.Marshal(v)
	if !ok || err != nil || string(data) != "[]" {
		t.Fatal(string(data), err)
	}
}

func FuzzOutputCapture(f *testing.F) {
	for _, seed := range []string{`{"value":"ok","secret":"hidden"}`, `{"value":"ok","nested":[{"private":"hidden","ok":true}]}`, `{}`, `null`, `{"value":false}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			return
		}
		var output map[string]any
		if json.Unmarshal(data, &output) != nil {
			return
		}
		budget := captureBudget{}
		value, ok := capture(output, registry.Field{Type: "object", Properties: descriptor().Outputs}, 0, &budget, true)
		if !ok {
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil || !json.Valid(encoded) {
			t.Fatal("capture produced invalid JSON", err)
		}
		visible := value.(map[string]any)
		if _, exists := visible["secret"]; exists {
			t.Fatal("sensitive field escaped")
		}
		if items, exists := visible["nested"]; exists {
			for _, item := range items.([]any) {
				if _, exists := item.(map[string]any)["private"]; exists {
					t.Fatal("nested sensitive field escaped")
				}
			}
		}
	})
}

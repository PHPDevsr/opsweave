package engine

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

func TestResolveInputsPreservesTypesAndCopies(t *testing.T) {
	steps := []execution.Step{{ID: "source", State: execution.Succeeded, Output: json.RawMessage(`{"count":3,"ratio":1.5,"items":[true,{"value":"hello"}]}`)}}
	ref := func(key string) workflow.Reference {
		return workflow.Reference{Kind: "output", Step: "source", Output: key}
	}
	inputs := map[string]any{"count": ref("count"), "ratio": ref("ratio"), "items": ref("items"), "nested": []any{map[string]any{"literal": "${not-interpolated}"}}}
	got, err := ResolveInputs(t.Context(), inputs, steps)
	if err != nil || got["count"] != int64(3) || got["ratio"] != 1.5 {
		t.Fatal(got, err)
	}
	got["items"].([]any)[1].(map[string]any)["value"] = "changed"
	got["nested"].([]any)[0].(map[string]any)["literal"] = "changed"
	if !strings.Contains(string(steps[0].Output), "hello") || inputs["nested"].([]any)[0].(map[string]any)["literal"] != "${not-interpolated}" {
		t.Fatal("shared input/output ownership")
	}
}

func TestResolveInputsRejectsUnavailableAndBounds(t *testing.T) {
	ref := workflow.Reference{Kind: "output", Step: "source", Output: "value"}
	for _, tc := range []struct {
		state execution.State
		data  string
	}{
		{execution.Failed, `{"value":"hidden"}`}, {execution.Succeeded, `{}`}, {execution.Succeeded, `bad`},
		{execution.Succeeded, `null`}, {execution.Succeeded, `{"value":null}`}, {execution.Succeeded, `{"value":1e999}`},
	} {
		_, err := ResolveInputs(t.Context(), map[string]any{"value": ref}, []execution.Step{{ID: "source", State: tc.state, Output: json.RawMessage(tc.data)}})
		if err == nil {
			t.Fatal(tc)
		}
	}
	for _, value := range []any{workflow.Reference{Kind: "secret"}, map[string]any{"x": strings.Repeat("x", 65537)}, math.NaN(), math.Inf(1), []any{nil}, json.Number("1e999")} {
		if _, err := ResolveInputs(t.Context(), map[string]any{"input": value}, nil); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	if _, err := ResolveInputs(t.Context(), cycle, nil); !errors.Is(err, execution.ErrLimit) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ResolveInputs(ctx, map[string]any{}, nil); err != context.Canceled {
		t.Fatal(err)
	}
	if _, err := ResolveInputs(t.Context(), nil, nil); err == nil {
		t.Fatal("nil input")
	}
	if _, err := ResolveInputs(t.Context(), map[string]any{"big": make([]any, 20001)}, nil); err != execution.ErrLimit {
		t.Fatal(err)
	}
}

func FuzzResolveOutput(f *testing.F) {
	for _, data := range []string{`{"value":1}`, `{"value":["ok"]}`, `null`, `{"value":null}`} {
		f.Add([]byte(data))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			return
		}
		result, err := ResolveInputs(t.Context(), map[string]any{"input": workflow.Reference{Kind: "output", Step: "a", Output: "value"}}, []execution.Step{{ID: "a", State: execution.Succeeded, Output: json.RawMessage(data)}})
		if err == nil {
			encoded, encodeErr := json.Marshal(result)
			if result == nil || encodeErr != nil || !json.Valid(encoded) {
				t.Fatal("resolved invalid JSON", encodeErr)
			}
		}
	})
}

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

// ResolveInputs copies literal/reference inputs without exposing unavailable or sensitive values.
// Registration must already have verified reference scope and non-sensitive output schemas.
func ResolveInputs(ctx context.Context, inputs map[string]any, steps []execution.Step) (map[string]any, error) {
	if len(steps) > 256 || len(inputs) > 20000 {
		return nil, execution.ErrLimit
	}
	r := inputResolver{ctx: ctx, steps: map[string]execution.Step{}, outputs: map[string]map[string]any{}}
	for _, step := range steps {
		r.steps[step.ID] = step
	}
	value, err := r.copy(inputs, 0)
	if err != nil {
		return nil, err
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, execution.ErrInvalid
	}
	return result, nil
}

type inputResolver struct {
	ctx                   context.Context
	steps                 map[string]execution.Step
	outputs               map[string]map[string]any
	nodes, bytes, decoded int
}

func (r *inputResolver) copy(value any, depth int) (any, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	r.nodes++
	if depth > 32 || r.nodes > 20000 || r.bytes > 65536 {
		return nil, execution.ErrLimit
	}
	switch v := value.(type) {
	case workflow.Reference:
		if v.Kind != "output" {
			return nil, execution.ErrReference
		}
		step, ok := r.steps[v.Step]
		if !ok || step.State != execution.Succeeded || len(step.Output) == 0 || len(step.Output) > 65536 {
			return nil, execution.ErrReference
		}
		out, ok := r.outputs[v.Step]
		if !ok {
			r.decoded += len(step.Output)
			if r.decoded > 1<<20 || !utf8.Valid(step.Output) {
				return nil, execution.ErrLimit
			}
			decoder := json.NewDecoder(bytes.NewReader(step.Output))
			decoder.UseNumber()
			if !json.Valid(step.Output) || decoder.Decode(&out) != nil || out == nil {
				return nil, execution.ErrReference
			}
			r.outputs[v.Step] = out
		}
		selected, ok := out[v.Output]
		if !ok {
			return nil, execution.ErrReference
		}
		return r.copy(selected, depth+1)
	case string:
		r.bytes += len(v)
		if r.bytes > 65536 || !utf8.ValidString(v) {
			return nil, execution.ErrLimit
		}
		return v, nil
	case bool, int64:
		return v, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, execution.ErrInvalid
		}
		return v, nil
	case json.Number:
		if integer, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return integer, nil
		}
		number, err := strconv.ParseFloat(string(v), 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, execution.ErrReference
		}
		return number, nil
	case map[string]any:
		if v == nil || len(v) > 20000 {
			return nil, execution.ErrInvalid
		}
		result := make(map[string]any, len(v))
		for key, child := range v {
			r.bytes += len(key)
			copied, err := r.copy(child, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = copied
		}
		return result, nil
	case []any:
		if len(v) > 20000 {
			return nil, execution.ErrLimit
		}
		result := make([]any, 0, len(v))
		for _, child := range v {
			copied, err := r.copy(child, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, copied)
		}
		return result, nil
	}
	return nil, execution.ErrReference
}

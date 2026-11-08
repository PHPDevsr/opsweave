package workflow

import (
	"context"
	"fmt"
)

func validateGraph(ctx context.Context, d *Definition) error {
	indices := make(map[string]int, len(d.Spec.Steps))
	for i, step := range d.Spec.Steps {
		if _, exists := indices[step.ID]; exists {
			return invalid(nil, fmt.Sprintf("spec.steps[%d].id", i), "duplicate_step")
		}
		indices[step.ID] = i
	}
	for i, step := range d.Spec.Steps {
		for _, dependency := range step.Needs {
			if _, exists := indices[dependency]; !exists || dependency == step.ID {
				return invalid(nil, fmt.Sprintf("spec.steps[%d].needs", i), "invalid_dependency")
			}
		}
	}
	finished := make(map[string]bool, len(indices))
	for len(finished) < len(indices) {
		if err := ctx.Err(); err != nil {
			return err
		}
		selected := -1
		for i, step := range d.Spec.Steps {
			if finished[step.ID] {
				continue
			}
			ready := true
			for _, dep := range step.Needs {
				ready = ready && finished[dep]
			}
			if ready {
				selected = i
				break
			}
		}
		if selected < 0 {
			return invalid(nil, "spec.steps", "dependency_cycle")
		}
		id := d.Spec.Steps[selected].ID
		finished[id] = true
		d.Order = append(d.Order, id)
	}
	for i, step := range d.Spec.Steps {
		ancestors := make(map[string]bool)
		if err := ctx.Err(); err != nil {
			return err
		}
		pending := append([]string(nil), step.Needs...)
		for len(pending) > 0 {
			id := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if !ancestors[id] {
				ancestors[id] = true
				pending = append(pending, d.Spec.Steps[indices[id]].Needs...)
			}
		}
		if step.Condition != "" {
			refs, err := ValidateCondition(step.Condition)
			if err != nil {
				return err
			}
			for _, id := range refs {
				if !ancestors[id] {
					return invalid(nil, fmt.Sprintf("spec.steps[%d].if", i), "reference_scope")
				}
			}
		}
		if !inputScope(step.Inputs, ancestors) {
			return invalid(nil, fmt.Sprintf("spec.steps[%d].with", i), "reference_scope")
		}
	}
	return ctx.Err()
}

func inputScope(value any, ancestors map[string]bool) bool {
	switch v := value.(type) {
	case Reference:
		return v.Kind != "output" || ancestors[v.Step]
	case map[string]any:
		for _, child := range v {
			if !inputScope(child, ancestors) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !inputScope(child, ancestors) {
				return false
			}
		}
	}
	return true
}

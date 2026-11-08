// Package workflow defines and validates candidate workflow definitions.
// Parsing does not authorize registration or execution.
package workflow

import (
	"fmt"
	"time"
)

// Definition is a structurally validated candidate, not an executable workflow.
// Action schemas and available runtime capabilities require separate validation.
type Definition struct {
	APIVersion string
	Kind       string
	Metadata   Metadata
	Spec       Spec
	// Order is a stable topological order, using authored step order for ties.
	Order  []string
	source []byte
}

// Source returns an independent copy of the original YAML, including comments.
func (d *Definition) Source() []byte { return append([]byte(nil), d.source...) }

// Metadata identifies and categorizes a workflow.
type Metadata struct {
	Name        string
	Description string
	Labels      map[string]string
}

// Spec contains normalized defaults and dependency edges.
type Spec struct {
	Steps       []Step
	Timeout     time.Duration
	Concurrency int
	Parallelism int
}

// Step defines one action invocation; Inputs contain literals or Reference values.
type Step struct {
	ID        string
	Uses      string
	Inputs    map[string]any
	Needs     []string
	Condition string
	Timeout   time.Duration
	Retry     Retry
}

// Retry bounds total attempts and exponential backoff.
type Retry struct {
	Attempts   int
	Backoff    time.Duration
	MaxBackoff time.Duration
}

// Reference preserves a symbolic input without resolving it or exposing a secret.
type Reference struct {
	Kind        string
	Step        string
	Output      string
	Environment string
	Name        string
}

// ValidationError reports a safe, bounded location and code, never input values.
// Path contains only schema field names and numeric indices.
type ValidationError struct {
	Code   string
	Path   string
	Line   int
	Column int
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("workflow %s at %s (line %d, column %d)", e.Code, e.Path, e.Line, e.Column)
}

// Phase01 checks definition-level capability availability, without action lookup.
// Passing this check does not substitute for action catalog/schema validation.
func (d *Definition) Phase01() error {
	if d.Spec.Timeout != time.Hour || d.Spec.Concurrency != 1 || d.Spec.Parallelism != 1 {
		return &ValidationError{Code: "unsupported_capability", Path: "spec"}
	}
	for i, s := range d.Spec.Steps {
		sequential := (i == 0 && len(s.Needs) == 0) || (i > 0 && len(s.Needs) == 1 && s.Needs[0] == d.Spec.Steps[i-1].ID)
		if !sequential || s.Condition != "" || s.Timeout != 5*time.Minute || s.Retry != (Retry{1, time.Second, time.Minute}) || hasReference(s.Inputs) {
			return &ValidationError{Code: "unsupported_capability", Path: fmt.Sprintf("spec.steps[%d]", i)}
		}
	}
	return nil
}

// SerialReliability permits reviewed execution policies while dispatcher capacity
// and fenced recovery remain unavailable. Secrets still require Phase 05.
func (d *Definition) SerialReliability() error {
	if d.Spec.Concurrency != 1 || d.Spec.Parallelism != 1 {
		return &ValidationError{Code: "unsupported_capability", Path: "spec"}
	}
	for i, step := range d.Spec.Steps {
		if hasSecretReference(step.Inputs) {
			return &ValidationError{Code: "unsupported_capability", Path: fmt.Sprintf("spec.steps[%d].with", i)}
		}
	}
	return nil
}

func hasSecretReference(value any) bool {
	switch v := value.(type) {
	case Reference:
		return v.Kind != "output"
	case map[string]any:
		for _, child := range v {
			if hasSecretReference(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if hasSecretReference(child) {
				return true
			}
		}
	}
	return false
}

func hasReference(value any) bool {
	switch v := value.(type) {
	case Reference:
		return true
	case map[string]any:
		for _, child := range v {
			if hasReference(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if hasReference(child) {
				return true
			}
		}
	}
	return false
}

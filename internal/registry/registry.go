// Package registry validates workflow deployments and defines persistence contracts.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/PHPDevsr/opsweave/internal/workflow"
)

// Field is validation metadata, not an executable action interface.
type Field struct {
	Type       string
	Required   bool
	Credential bool
	Sensitive  bool
	Properties map[string]Field
	Items      *Field
}

// Descriptor binds validation to a particular compatible action version.
type Descriptor struct {
	ID, Version, APIVersion string
	Inputs                  Field
	Outputs                 map[string]Field
	Permissions             []string
	Capabilities            []string
}

// Catalog is supplied by application composition. Production is empty in Phase 01.
type Catalog map[string]Descriptor

// Source is one bounded input document. Paths never become stored metadata.
type Source struct{ Data []byte }

// Candidate contains validated revision content for an atomic apply batch.
type Candidate struct {
	Name         string            `json:"name"`
	Source       []byte            `json:"-"`
	Normalized   json.RawMessage   `json:"definition"`
	Bindings     map[string]string `json:"actions"`
	Order        []string          `json:"order"`
	Capabilities []string          `json:"capabilities"`
}

// Record identifies a workflow revision and its current lifecycle state.
type Record struct {
	Name       string          `json:"name"`
	Revision   string          `json:"revision"`
	Number     int             `json:"number"`
	Enabled    bool            `json:"enabled"`
	Archived   bool            `json:"archived"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	Definition json.RawMessage `json:"definition,omitempty"`
	Bindings   json.RawMessage `json:"actions,omitempty"`
	Source     []byte          `json:"-"`
}

// Repository implements atomic storage without coupling domain services to SQL.
type Repository interface {
	Apply(context.Context, []Candidate) ([]Record, error)
	List(context.Context, bool) ([]Record, error)
	Get(context.Context, string, string) (Record, error)
	Revisions(context.Context, string) ([]Record, error)
	SetEnabled(context.Context, string, bool) (Record, error)
	Archive(context.Context, string) (Record, error)
}

// ErrNotFound and ErrArchived distinguish lifecycle failures from infrastructure.
var (
	ErrNotFound    = errors.New("workflow or revision not found")
	ErrArchived    = errors.New("workflow is archived")
	ErrResultLimit = errors.New("workflow listing exceeds response limit; use exact get or export")
)

// Prepare validates a complete batch without any storage or external side effects.
func Prepare(ctx context.Context, sources []Source, catalog Catalog) ([]Candidate, error) {
	return prepare(ctx, sources, catalog, 0)
}

// PrepareReliable validates the reviewed serial reliability capability set.
func PrepareReliable(ctx context.Context, sources []Source, catalog Catalog) ([]Candidate, error) {
	return prepare(ctx, sources, catalog, 1)
}

// PrepareDispatcher validates bounded worker capabilities without authorizing an action invocation.
func PrepareDispatcher(ctx context.Context, sources []Source, catalog Catalog) ([]Candidate, error) {
	return prepare(ctx, sources, catalog, 2)
}

func prepare(ctx context.Context, sources []Source, catalog Catalog, capability int) ([]Candidate, error) {
	if len(sources) == 0 || len(sources) > 256 {
		return nil, errors.New("batch must contain 1 to 256 workflows")
	}
	result := make([]Candidate, 0, len(sources))
	seen := make(map[string]bool)
	total := 0
	for _, source := range sources {
		total += len(source.Data)
		if total > 16<<20 {
			return nil, errors.New("workflow batch exceeds size limit")
		}
		d, err := workflow.Parse(ctx, source.Data)
		if err != nil {
			return nil, err
		}
		if seen[d.Metadata.Name] {
			return nil, errors.New("duplicate workflow name in batch")
		}
		seen[d.Metadata.Name] = true
		bindings, err := validateActions(d, catalog)
		if err != nil {
			return nil, err
		}
		capabilityErr := d.Phase01()
		if capability > 0 {
			capabilityErr = d.SerialReliability()
			if capability == 2 {
				capabilityErr = d.DispatcherCapabilities()
			}
		}
		if capabilityErr != nil {
			return nil, capabilityErr
		}
		normalized, err := Normalize(d)
		if err != nil {
			return nil, fmt.Errorf("normalize workflow: %w", err)
		}
		result = append(result, Candidate{Name: d.Metadata.Name, Source: d.Source(), Normalized: normalized, Bindings: bindings, Order: append([]string(nil), d.Order...), Capabilities: []string{}})
		if capability > 0 {
			result[len(result)-1].Capabilities = []string{"serial_reliability"}
			if capability == 2 {
				result[len(result)-1].Capabilities = []string{"dispatcher"}
			}
		}
	}
	return result, ctx.Err()
}

func validateActions(d *workflow.Definition, catalog Catalog) (map[string]string, error) {
	bindings := make(map[string]string)
	steps := make(map[string]Descriptor)
	for _, step := range d.Spec.Steps {
		descriptor, exists := catalog[step.Uses]
		if !exists {
			return nil, errors.New("action unavailable: production catalog contains no built-in actions")
		}
		if descriptor.ID != step.Uses || descriptor.APIVersion != d.APIVersion || descriptor.Version == "" || !schemaValid(descriptor.Inputs, 0) || descriptor.Inputs.Type != "object" {
			return nil, errors.New("invalid or incompatible action descriptor")
		}
		for _, field := range descriptor.Outputs {
			if !schemaValid(field, 0) {
				return nil, errors.New("invalid action output descriptor")
			}
		}
		if len(descriptor.Capabilities) > 0 {
			return nil, errors.New("unsupported action capability")
		}
		steps[step.ID] = descriptor
		bindings[descriptor.ID] = descriptor.Version
	}
	for _, step := range d.Spec.Steps {
		if !matches(step.Inputs, steps[step.ID].Inputs, steps, 0) {
			return nil, errors.New("action input schema or credential validation failed")
		}
	}
	return bindings, nil
}

func schemaValid(f Field, depth int) bool {
	if depth > 32 || (f.Credential && f.Type != "string") {
		return false
	}
	switch f.Type {
	case "string", "boolean", "integer", "number":
		return f.Items == nil && len(f.Properties) == 0
	case "array":
		return f.Items != nil && len(f.Properties) == 0 && schemaValid(*f.Items, depth+1)
	case "object":
		if f.Items != nil || len(f.Properties) > 20000 {
			return false
		}
		for _, child := range f.Properties {
			if !schemaValid(child, depth+1) {
				return false
			}
		}
		return true
	}
	return false
}

// ValidDescriptor checks the common metadata contract for trusted compiled actions.
func ValidDescriptor(d Descriptor) bool {
	if !workflow.ValidActionID(d.ID) || d.Version == "" || d.APIVersion != "opsweave.io/v1" || d.Inputs.Type != "object" || !schemaValid(d.Inputs, 0) || len(d.Capabilities) > 0 {
		return false
	}
	for _, f := range d.Outputs {
		if !schemaValid(f, 0) {
			return false
		}
	}
	return true
}

func matches(value any, f Field, steps map[string]Descriptor, depth int) bool {
	if depth > 32 {
		return false
	}
	if ref, ok := value.(workflow.Reference); ok {
		if f.Credential {
			return ref.Kind == "secret" && f.Type == "string"
		}
		if ref.Kind != "output" {
			return false
		}
		out, exists := steps[ref.Step].Outputs[ref.Output]
		return exists && !sensitive(out, 0) && sameType(f, out, 0)
	}
	if f.Credential {
		return false
	}
	switch f.Type {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		_, ok := value.(int64)
		return ok
	case "number":
		switch value.(type) {
		case int64, float64:
			return true
		}
	case "object":
		v, ok := value.(map[string]any)
		if !ok {
			return false
		}
		for key, field := range f.Properties {
			if _, exists := v[key]; field.Required && !exists {
				return false
			}
		}
		for key, child := range v {
			field, exists := f.Properties[key]
			if !exists || !matches(child, field, steps, depth+1) {
				return false
			}
		}
		return true
	case "array":
		v, ok := value.([]any)
		if !ok || f.Items == nil {
			return false
		}
		for _, child := range v {
			if !matches(child, *f.Items, steps, depth+1) {
				return false
			}
		}
		return true
	}
	return false
}

func sensitive(f Field, depth int) bool {
	if depth > 32 || f.Sensitive || f.Credential {
		return true
	}
	if f.Items != nil && sensitive(*f.Items, depth+1) {
		return true
	}
	for _, child := range f.Properties {
		if sensitive(child, depth+1) {
			return true
		}
	}
	return false
}

func sameType(a, b Field, depth int) bool {
	if depth > 32 || a.Type != b.Type || a.Credential != b.Credential {
		return false
	}
	if a.Type == "array" {
		return a.Items != nil && b.Items != nil && sameType(*a.Items, *b.Items, depth+1)
	}
	if a.Type == "object" {
		if len(a.Properties) != len(b.Properties) {
			return false
		}
		for key, field := range a.Properties {
			other, exists := b.Properties[key]
			if !exists || field.Required != other.Required || !sameType(field, other, depth+1) {
				return false
			}
		}
	}
	return true
}

// Normalize produces version-1 deterministic semantic JSON with string durations.
func Normalize(d *workflow.Definition) ([]byte, error) {
	steps := make([]any, 0, len(d.Spec.Steps))
	for _, s := range d.Spec.Steps {
		needs := append([]string{}, s.Needs...)
		steps = append(steps, map[string]any{"id": s.ID, "uses": s.Uses, "with": normalizeInput(s.Inputs), "needs": needs, "if": s.Condition, "timeout": s.Timeout.String(), "retry": map[string]any{"attempts": s.Retry.Attempts, "backoff": s.Retry.Backoff.String(), "maxBackoff": s.Retry.MaxBackoff.String()}})
	}
	spec := map[string]any{"steps": steps, "timeout": d.Spec.Timeout.String(), "concurrency": map[string]int{"limit": d.Spec.Concurrency}, "parallelism": d.Spec.Parallelism}
	if len(d.Spec.Triggers) > 0 {
		spec["triggers"] = d.Spec.Triggers
	}
	return json.Marshal(map[string]any{"apiVersion": d.APIVersion, "kind": d.Kind, "metadata": map[string]any{"name": d.Metadata.Name, "description": d.Metadata.Description, "labels": d.Metadata.Labels}, "spec": spec})
}

func normalizeInput(value any) any {
	switch v := value.(type) {
	case workflow.Reference:
		if v.Kind == "secret" {
			return map[string]string{"$ref": "secrets." + v.Environment + "." + v.Name}
		}
		return map[string]string{"$ref": "steps." + v.Step + ".outputs." + v.Output}
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, child := range v {
			result[key] = normalizeInput(child)
		}
		return result
	case []any:
		result := make([]any, 0, len(v))
		for _, child := range v {
			result = append(result, normalizeInput(child))
		}
		return result
	}
	return value
}

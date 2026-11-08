// Package action executes trusted, compiled implementations with bounded typed capture.
package action

import (
	"context"
	"encoding/json"
	"math"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
)

// Handler must honor context cancellation and bound its own external I/O and allocations.
type Handler func(context.Context, map[string]any) (map[string]any, error)

// Registration binds one exact action version to its validation metadata and implementation.
type Registration struct {
	Descriptor registry.Descriptor
	Run        Handler
}

// Runtime owns immutable copies of registration metadata and explicit permission grants.
type Runtime struct {
	entries map[string]Registration
	grants  map[string]bool
}

// New validates and snapshots trusted composition; it does not load plugins or grant wildcards.
func New(entries []Registration, grants []string) (*Runtime, error) {
	r := &Runtime{entries: map[string]Registration{}, grants: map[string]bool{}}
	for _, g := range grants {
		if g == "" || g == "*" {
			return nil, execution.ErrPermission
		}
		r.grants[g] = true
	}
	for _, entry := range entries {
		b, err := json.Marshal(entry.Descriptor)
		if err != nil || len(b) > 1<<20 {
			return nil, execution.ErrInvalid
		}
		var d registry.Descriptor
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, execution.ErrInvalid
		}
		if !registry.ValidDescriptor(d) || entry.Run == nil {
			return nil, execution.ErrInvalid
		}
		key := d.ID + "\x00" + d.Version
		if _, ok := r.entries[key]; ok {
			return nil, execution.ErrInvalid
		}
		for _, p := range d.Permissions {
			if p == "" || p == "*" {
				return nil, execution.ErrPermission
			}
		}
		r.entries[key] = Registration{d, entry.Run}
	}
	return r, nil
}

// Check enforces exact-version availability and explicit permissions without invocation.
func (r *Runtime) Check(id, version string) error {
	if r == nil {
		return execution.ErrUnavailable
	}
	e, ok := r.entries[id+"\x00"+version]
	if !ok {
		return execution.ErrUnavailable
	}
	for _, p := range e.Descriptor.Permissions {
		if !r.grants[p] {
			return execution.ErrPermission
		}
	}
	return nil
}

// Catalog returns independent descriptors for exact immutable revision bindings.
func (r *Runtime) Catalog(bindings map[string]string) (registry.Catalog, error) {
	catalog := registry.Catalog{}
	for id, version := range bindings {
		if err := r.Check(id, version); err != nil {
			return nil, err
		}
		data, err := json.Marshal(r.entries[id+"\x00"+version].Descriptor)
		if err != nil {
			return nil, execution.ErrInvalid
		}
		var d registry.Descriptor
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, execution.ErrInvalid
		}
		catalog[id] = d
	}
	return catalog, nil
}

// Invoke revalidates inputs, observes context, and returns only schema-checked redacted JSON.
// Handler error text is deliberately discarded at this security boundary.
func (r *Runtime) Invoke(ctx context.Context, id, version string, inputs map[string]any) (json.RawMessage, error) {
	if err := r.Check(id, version); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e := r.entries[id+"\x00"+version]
	budget := captureBudget{}
	copied, ok := capture(inputs, e.Descriptor.Inputs, 0, &budget, false)
	if !ok {
		return nil, execution.ErrInvalid
	}
	output, err := e.Run(ctx, copied.(map[string]any))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, execution.ErrAction
	}
	budget = captureBudget{}
	redacted, ok := capture(output, registry.Field{Type: "object", Properties: e.Descriptor.Outputs}, 0, &budget, true)
	if !ok {
		return nil, execution.ErrOutput
	}
	data, err := json.Marshal(redacted)
	if err != nil || len(data) > 65536 {
		return nil, execution.ErrOutput
	}
	return data, nil
}

type captureBudget struct{ nodes, bytes int }

// capture validates even omitted sensitive descendants and copies only allowed values.
func capture(v any, f registry.Field, depth int, b *captureBudget, redact bool) (any, bool) {
	b.nodes++
	if depth > 32 || b.nodes > 20000 || b.bytes > 65536 || (!redact && f.Credential) {
		return nil, false
	}
	var result any
	switch f.Type {
	case "string":
		s, ok := v.(string)
		if !ok || !utf8.ValidString(s) {
			return nil, false
		}
		b.bytes += len(s)
		result = s
	case "boolean":
		x, ok := v.(bool)
		if !ok {
			return nil, false
		}
		result = x
	case "integer":
		x, ok := v.(int64)
		if !ok {
			return nil, false
		}
		result = x
	case "number":
		switch x := v.(type) {
		case int64:
			result = x
		case float64:
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return nil, false
			}
			result = x
		default:
			return nil, false
		}
	case "object":
		x, ok := v.(map[string]any)
		if !ok || x == nil {
			return nil, false
		}
		out := map[string]any{}
		for key, field := range f.Properties {
			if _, exists := x[key]; field.Required && !exists {
				return nil, false
			}
		}
		for key, value := range x {
			b.bytes += len(key)
			field, exists := f.Properties[key]
			if !exists {
				return nil, false
			}
			child, ok := capture(value, field, depth+1, b, redact)
			if !ok {
				return nil, false
			}
			if !redact || (!field.Sensitive && !field.Credential) {
				out[key] = child
			}
		}
		result = out
	case "array":
		x, ok := v.([]any)
		if !ok || f.Items == nil {
			return nil, false
		}
		out := make([]any, 0)
		for _, value := range x {
			child, ok := capture(value, *f.Items, depth+1, b, redact)
			if !ok {
				return nil, false
			}
			if !redact || (!f.Items.Sensitive && !f.Items.Credential) {
				out = append(out, child)
			}
		}
		result = out
	default:
		return nil, false
	}
	return result, b.bytes <= 65536
}

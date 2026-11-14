package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PHPDevsr/opsweave/internal/yamlutil"
	"go.yaml.in/yaml/v3"
)

// MaxSourceBytes bounds a complete workflow YAML document.
const MaxSourceBytes = 1 << 20

// Parse validates YAML syntax, schema, dependency and reference structure.
// It preserves the original source and applies defaults. Action availability,
// output schemas and credentials need catalog validation before registration.
func Parse(ctx context.Context, source []byte) (*Definition, error) {
	root, err := yamlutil.Load(ctx, source, MaxSourceBytes)
	if err != nil {
		var validation *yamlutil.Error
		if errors.As(err, &validation) {
			return nil, &ValidationError{Code: validation.Code, Path: "$", Line: validation.Line, Column: validation.Column}
		}
		return nil, err
	}
	d, err := parseDefinition(root)
	if err != nil {
		return nil, err
	}
	if err := validateGraph(ctx, d); err != nil {
		return nil, err
	}
	d.source = bytes.Clone(source)
	return d, nil
}

func invalid(n *yaml.Node, path, code string) error {
	e := &ValidationError{Code: code, Path: path}
	if n != nil {
		e.Line, e.Column = n.Line, n.Column
	}
	return e
}

func fields(n *yaml.Node, path string, allowed ...string) (map[string]*yaml.Node, error) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, invalid(n, path, "expected_mapping")
	}
	result := make(map[string]*yaml.Node, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		known := false
		for _, name := range allowed {
			known = known || key == name
		}
		if !known {
			// An unknown key may itself contain sensitive data.
			return nil, invalid(n.Content[i], path, "unknown_field")
		}
		result[key] = n.Content[i+1]
	}
	return result, nil
}

func textValue(n *yaml.Node, path string) (string, error) {
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
		return "", invalid(n, path, "expected_string")
	}
	return n.Value, nil
}

func identifier(s string) bool {
	if len(s) < 1 || len(s) > 63 || s[0] < 'a' || s[0] > 'z' || s[len(s)-1] == '-' {
		return false
	}
	for i, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			continue
		}
		if c != '-' || i == 0 || s[i-1] == '-' {
			return false
		}
	}
	return true
}

func idValue(n *yaml.Node, path string) (string, error) {
	value, err := textValue(n, path)
	if err != nil {
		return "", err
	}
	if !identifier(value) {
		return "", invalid(n, path, "invalid_identifier")
	}
	return value, nil
}

func parseDefinition(n *yaml.Node) (*Definition, error) {
	f, err := fields(n, "$", "apiVersion", "kind", "metadata", "spec")
	if err != nil {
		return nil, err
	}
	version, err := textValue(f["apiVersion"], "apiVersion")
	if err != nil {
		return nil, err
	}
	if version != "opsweave.io/v1" {
		return nil, invalid(f["apiVersion"], "apiVersion", "unsupported_version")
	}
	kind, err := textValue(f["kind"], "kind")
	if err != nil {
		return nil, err
	}
	if kind != "Workflow" {
		return nil, invalid(f["kind"], "kind", "invalid_kind")
	}
	m, err := parseMetadata(f["metadata"])
	if err != nil {
		return nil, err
	}
	spec, err := parseSpec(f["spec"])
	if err != nil {
		return nil, err
	}
	return &Definition{APIVersion: version, Kind: kind, Metadata: m, Spec: spec}, nil
}

func parseMetadata(n *yaml.Node) (Metadata, error) {
	m := Metadata{Labels: make(map[string]string)}
	f, err := fields(n, "metadata", "name", "description", "labels")
	if err != nil {
		return m, err
	}
	if m.Name, err = idValue(f["name"], "metadata.name"); err != nil {
		return m, err
	}
	if v := f["description"]; v != nil {
		if m.Description, err = textValue(v, "metadata.description"); err != nil {
			return m, err
		}
		if len(m.Description) > 4096 {
			return m, invalid(v, "metadata.description", "resource_limit")
		}
	}
	if v := f["labels"]; v != nil {
		if v.Kind != yaml.MappingNode || len(v.Content)/2 > 256 {
			return m, invalid(v, "metadata.labels", "invalid_labels")
		}
		for i := 0; i < len(v.Content); i += 2 {
			key, err := idValue(v.Content[i], "metadata.labels")
			if err != nil {
				return m, err
			}
			value, err := textValue(v.Content[i+1], "metadata.labels")
			if err != nil {
				return m, err
			}
			if len(value) > 256 {
				return m, invalid(v, "metadata.labels", "resource_limit")
			}
			m.Labels[key] = value
		}
	}
	return m, nil
}

func boundedInt(n *yaml.Node, path string, fallback, max int) (int, error) {
	if n == nil {
		return fallback, nil
	}
	var value int
	if n.Kind != yaml.ScalarNode || n.Tag != "!!int" || n.Decode(&value) != nil || value < 1 || value > max {
		return 0, invalid(n, path, "invalid_integer")
	}
	return value, nil
}

func duration(n *yaml.Node, path string, fallback time.Duration) (time.Duration, error) {
	if n == nil {
		return fallback, nil
	}
	value, err := textValue(n, path)
	if err != nil {
		return 0, err
	}
	i := 0
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
	}
	multiplier := time.Duration(0)
	switch value[i:] {
	case "ms":
		multiplier = time.Millisecond
	case "s":
		multiplier = time.Second
	case "m":
		multiplier = time.Minute
	case "h":
		multiplier = time.Hour
	}
	amount, parseErr := strconv.ParseUint(value[:i], 10, 64)
	if parseErr != nil || multiplier == 0 || amount == 0 || amount > uint64(24*time.Hour)/uint64(multiplier) {
		return 0, invalid(n, path, "invalid_duration")
	}
	return time.Duration(amount) * multiplier, nil
}

func parseSpec(n *yaml.Node) (Spec, error) {
	s := Spec{Concurrency: 1}
	f, err := fields(n, "spec", "steps", "timeout", "concurrency", "parallelism", "triggers")
	if err != nil {
		return s, err
	}
	if s.Triggers, err = parseTriggers(f["triggers"]); err != nil {
		return s, err
	}
	if s.Timeout, err = duration(f["timeout"], "spec.timeout", time.Hour); err != nil {
		return s, err
	}
	if s.Parallelism, err = boundedInt(f["parallelism"], "spec.parallelism", 1, 256); err != nil {
		return s, err
	}
	if v := f["concurrency"]; v != nil {
		c, err := fields(v, "spec.concurrency", "limit")
		if err != nil {
			return s, err
		}
		if s.Concurrency, err = boundedInt(c["limit"], "spec.concurrency.limit", 1, 256); err != nil {
			return s, err
		}
	}
	v := f["steps"]
	if v == nil || v.Kind != yaml.SequenceNode || len(v.Content) == 0 || len(v.Content) > 256 {
		return s, invalid(v, "spec.steps", "invalid_steps")
	}
	for i, node := range v.Content {
		step, err := parseStep(node, i)
		if err != nil {
			return s, err
		}
		if step.Needs == nil && i > 0 {
			step.Needs = []string{s.Steps[i-1].ID}
		}
		s.Steps = append(s.Steps, step)
	}
	return s, nil
}

func parseStep(n *yaml.Node, index int) (Step, error) {
	s := Step{Inputs: make(map[string]any)}
	path := fmt.Sprintf("spec.steps[%d]", index)
	f, err := fields(n, path, "id", "uses", "with", "needs", "if", "timeout", "retry")
	if err != nil {
		return s, err
	}
	if s.ID, err = idValue(f["id"], path+".id"); err != nil {
		return s, err
	}
	if s.Uses, err = textValue(f["uses"], path+".uses"); err != nil {
		return s, err
	}
	parts := strings.Split(s.Uses, ".")
	if len(parts) != 2 || !actionSegment(parts[0]) || !actionSegment(parts[1]) {
		return s, invalid(f["uses"], path+".uses", "invalid_action")
	}
	if s.Timeout, err = duration(f["timeout"], path+".timeout", 5*time.Minute); err != nil {
		return s, err
	}
	if s.Retry, err = parseRetry(f["retry"], path+".retry"); err != nil {
		return s, err
	}
	if v := f["if"]; v != nil {
		if s.Condition, err = textValue(v, path+".if"); err != nil {
			return s, err
		}
		if _, err := ValidateCondition(s.Condition); err != nil {
			return s, invalid(v, path+".if", "invalid_condition")
		}
	}
	if v := f["needs"]; v != nil {
		if v.Kind != yaml.SequenceNode || len(v.Content) > 64 {
			return s, invalid(v, path+".needs", "invalid_dependencies")
		}
		s.Needs = make([]string, 0, len(v.Content))
		seen := make(map[string]bool)
		for _, dep := range v.Content {
			id, err := idValue(dep, path+".needs")
			if err != nil {
				return s, err
			}
			if seen[id] {
				return s, invalid(dep, path+".needs", "duplicate_dependency")
			}
			seen[id] = true
			s.Needs = append(s.Needs, id)
		}
	}
	if v := f["with"]; v != nil {
		if v.Kind != yaml.MappingNode {
			return s, invalid(v, path+".with", "expected_mapping")
		}
		value, err := input(v, path+".with")
		if err != nil {
			return s, err
		}
		mapping, ok := value.(map[string]any)
		if !ok {
			return s, invalid(v, path+".with", "expected_mapping")
		}
		s.Inputs = mapping
	}
	return s, nil
}

func actionSegment(value string) bool {
	return identifier(strings.ReplaceAll(value, "_", "-"))
}

// ValidActionID reports whether an action identifier follows the reviewed two-segment grammar.
func ValidActionID(value string) bool {
	parts := strings.Split(value, ".")
	return len(parts) == 2 && actionSegment(parts[0]) && actionSegment(parts[1])
}

func parseRetry(n *yaml.Node, path string) (Retry, error) {
	r := Retry{1, time.Second, time.Minute}
	if n == nil {
		return r, nil
	}
	f, err := fields(n, path, "attempts", "backoff", "maxBackoff")
	if err != nil {
		return r, err
	}
	if r.Attempts, err = boundedInt(f["attempts"], path+".attempts", 1, 10); err != nil {
		return r, err
	}
	if r.Backoff, err = duration(f["backoff"], path+".backoff", time.Second); err != nil {
		return r, err
	}
	if r.MaxBackoff, err = duration(f["maxBackoff"], path+".maxBackoff", time.Minute); err != nil {
		return r, err
	}
	if r.MaxBackoff < r.Backoff {
		return r, invalid(n, path, "invalid_backoff")
	}
	return r, nil
}

func input(n *yaml.Node, path string) (any, error) {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			if n.Content[i].Value == "$ref" {
				if len(n.Content) != 2 {
					return nil, invalid(n, path, "invalid_reference")
				}
				raw, err := textValue(n.Content[i+1], path)
				if err != nil {
					return nil, err
				}
				return reference(raw, n, path)
			}
		}
		result := make(map[string]any, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			// Do not include untrusted action input keys in diagnostics.
			value, err := input(n.Content[i+1], path)
			if err != nil {
				return nil, err
			}
			result[n.Content[i].Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(n.Content))
		for _, child := range n.Content {
			value, err := input(child, path)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	default:
		if n.Tag == "!!int" {
			var value int64
			if err := n.Decode(&value); err != nil {
				return nil, invalid(n, path, "invalid_number")
			}
			return value, nil
		}
		var value any
		if err := n.Decode(&value); err != nil {
			return nil, invalid(n, path, "invalid_scalar")
		}
		return value, nil
	}
}

func reference(raw string, n *yaml.Node, path string) (Reference, error) {
	p := strings.Split(raw, ".")
	if len(p) == 4 && p[0] == "steps" && identifier(p[1]) && p[2] == "outputs" && identifier(p[3]) {
		return Reference{Kind: "output", Step: p[1], Output: p[3]}, nil
	}
	if len(p) == 3 && p[0] == "secrets" && identifier(p[1]) && identifier(p[2]) {
		return Reference{Kind: "secret", Environment: p[1], Name: p[2]}, nil
	}
	return Reference{}, invalid(n, path, "invalid_reference")
}

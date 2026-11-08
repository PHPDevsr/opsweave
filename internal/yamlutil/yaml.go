// Package yamlutil provides bounded, strict YAML node loading with safe errors.
package yamlutil

import (
	"bytes"
	"context"
	"fmt"
	"go.yaml.in/yaml/v3"
	"io"
	"math"
	"unicode/utf8"
)

// Error omits input values and raw decoder errors.
type Error struct {
	Code         string
	Line, Column int
}

func (e *Error) Error() string {
	return fmt.Sprintf("YAML %s (line %d, column %d)", e.Code, e.Line, e.Column)
}
func failure(n *yaml.Node, code string) error {
	e := &Error{Code: code}
	if n != nil {
		e.Line, e.Column = n.Line, n.Column
	}
	return e
}

// Load returns one audited mapping/sequence/scalar root. Callers validate schema.
func Load(ctx context.Context, source []byte, maxBytes int) (*yaml.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(source) > maxBytes || !utf8.Valid(source) {
		return nil, failure(nil, "invalid_source")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil || len(root.Content) != 1 {
		return nil, failure(nil, "invalid_yaml")
	}
	count := 0
	if err := audit(ctx, root.Content[0], 0, &count); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, failure(nil, "multiple_documents")
	}
	return root.Content[0], nil
}

func audit(ctx context.Context, n *yaml.Node, depth int, count *int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	*count++
	if depth > 32 || *count > 20000 {
		return failure(n, "resource_limit")
	}
	if n.Anchor != "" || n.Kind == yaml.AliasNode || n.Style&yaml.TaggedStyle != 0 {
		return failure(n, "unsupported_yaml")
	}
	switch n.Kind {
	case yaml.MappingNode:
		keys := make(map[string]bool, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" || keys[key.Value] {
				return failure(key, "invalid_mapping_key")
			}
			keys[key.Value] = true
		}
	case yaml.SequenceNode:
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!str", "!!bool":
		case "!!timestamp":
			// Dates remain literal strings, never implicit timestamp values.
			n.Tag = "!!str"
		case "!!int":
			var value int64
			if err := n.Decode(&value); err != nil {
				return failure(n, "invalid_number")
			}
		case "!!float":
			var value float64
			if err := n.Decode(&value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return failure(n, "invalid_number")
			}
		default:
			return failure(n, "invalid_scalar")
		}
	default:
		return failure(n, "unsupported_yaml")
	}
	for _, child := range n.Content {
		if err := audit(ctx, child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

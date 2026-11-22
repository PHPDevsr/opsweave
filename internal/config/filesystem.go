package config

import (
	"errors"
	"strconv"

	"github.com/PHPDevsr/opsweave/internal/fileaction"
	"go.yaml.in/yaml/v3"
)

func decodeFilesystem(n *yaml.Node, c *fileaction.Config) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("invalid filesystem configuration")
	}
	for i := 0; i < len(n.Content); i += 2 {
		key, v := n.Content[i].Value, n.Content[i+1]
		switch key {
		case "enabled":
			if v.Kind != yaml.ScalarNode || v.Tag != "!!bool" || (v.Value != "true" && v.Value != "false") {
				return errors.New("invalid filesystem enabled setting")
			}
			c.Enabled = v.Value == "true"
		case "max_copy_bytes":
			if v.Kind != yaml.ScalarNode || v.Tag != "!!int" {
				return errors.New("invalid filesystem copy limit")
			}
			value, err := strconv.ParseInt(v.Value, 10, 64)
			if err != nil || value < 1<<20 || value > 64<<20 {
				return errors.New("invalid filesystem copy limit")
			}
			c.MaxCopyBytes = value
		case "roots":
			if v.Kind != yaml.SequenceNode || len(v.Content) > 128 {
				return errors.New("invalid filesystem roots")
			}
			for _, node := range v.Content {
				if node.Kind != yaml.MappingNode {
					return errors.New("invalid filesystem root")
				}
				var root fileaction.Root
				for j := 0; j < len(node.Content); j += 2 {
					field, value := node.Content[j].Value, node.Content[j+1]
					if field == "allowed_actions" {
						if value.Kind != yaml.SequenceNode || len(value.Content) > 3 {
							return errors.New("invalid filesystem root grants")
						}
						for _, id := range value.Content {
							if id.Kind != yaml.ScalarNode || id.Tag != "!!str" {
								return errors.New("invalid filesystem root grant")
							}
							root.AllowedActions = append(root.AllowedActions, id.Value)
						}
						continue
					}
					if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
						return errors.New("invalid filesystem root field")
					}
					switch field {
					case "name":
						root.Name = value.Value
					case "path":
						root.Path = value.Value
					default:
						return errors.New("unknown filesystem root field")
					}
				}
				c.Roots = append(c.Roots, root)
			}
		default:
			return errors.New("unknown filesystem configuration setting")
		}
	}
	return nil
}

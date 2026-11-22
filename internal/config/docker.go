package config

import (
	"errors"

	"github.com/PHPDevsr/opsweave/internal/dockeraction"
	"go.yaml.in/yaml/v3"
)

func decodeDocker(n *yaml.Node, c *dockeraction.Config) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("invalid Docker configuration")
	}
	for i := 0; i < len(n.Content); i += 2 {
		key, v := n.Content[i].Value, n.Content[i+1]
		switch key {
		case "enabled":
			if v.Kind != yaml.ScalarNode || v.Tag != "!!bool" || (v.Value != "true" && v.Value != "false") {
				return errors.New("invalid Docker enabled setting")
			}
			c.Enabled = v.Value == "true"
		case "endpoint":
			if v.Kind != yaml.ScalarNode || v.Tag != "!!str" {
				return errors.New("invalid Docker endpoint setting")
			}
			c.Endpoint = v.Value
		case "allowed_containers", "allowed_actions":
			limit := 128
			if key == "allowed_actions" {
				limit = 4
			}
			if v.Kind != yaml.SequenceNode || len(v.Content) > limit {
				return errors.New("invalid Docker allowlist")
			}
			var values []string
			for _, value := range v.Content {
				if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
					return errors.New("invalid Docker allowlist entry")
				}
				values = append(values, value.Value)
			}
			if key == "allowed_containers" {
				c.AllowedContainers = values
			} else {
				c.AllowedActions = values
			}
		default:
			return errors.New("unknown Docker configuration setting")
		}
	}
	return nil
}

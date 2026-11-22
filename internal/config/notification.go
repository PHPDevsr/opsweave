package config

import (
	"errors"
	"strconv"

	"github.com/PHPDevsr/opsweave/internal/notificationaction"
	"go.yaml.in/yaml/v3"
)

func decodeNotification(n *yaml.Node, c *notificationaction.Config) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("invalid notification configuration")
	}
	for i := 0; i < len(n.Content); i += 2 {
		key, v := n.Content[i].Value, n.Content[i+1]
		switch key {
		case "enabled":
			if v.Kind != yaml.ScalarNode || v.Tag != "!!bool" || (v.Value != "true" && v.Value != "false") {
				return errors.New("invalid notification enabled setting")
			}
			c.Enabled = v.Value == "true"
		case "profiles":
			if v.Kind != yaml.SequenceNode || len(v.Content) > 128 {
				return errors.New("invalid notification profiles")
			}
			for _, node := range v.Content {
				p, err := decodeNotificationProfile(node)
				if err != nil {
					return err
				}
				c.Profiles = append(c.Profiles, p)
			}
		default:
			return errors.New("unknown notification configuration setting")
		}
	}
	return nil
}

func decodeNotificationProfile(n *yaml.Node) (notificationaction.Profile, error) {
	var p notificationaction.Profile
	if n.Kind != yaml.MappingNode {
		return p, errors.New("invalid notification profile")
	}
	for i := 0; i < len(n.Content); i += 2 {
		key, v := n.Content[i].Value, n.Content[i+1]
		if key == "recipients" || key == "allowed_private_cidrs" {
			limit := 128
			if key == "recipients" {
				limit = 10
			}
			if v.Kind != yaml.SequenceNode || len(v.Content) > limit {
				return p, errors.New("invalid notification profile list")
			}
			var values []string
			for _, item := range v.Content {
				if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
					return p, errors.New("invalid notification list entry")
				}
				values = append(values, item.Value)
			}
			if key == "recipients" {
				p.Recipients = values
			} else {
				p.AllowedPrivateCIDRs = values
			}
			continue
		}
		if key == "port" {
			if v.Kind != yaml.ScalarNode || v.Tag != "!!int" {
				return p, errors.New("invalid SMTP port")
			}
			value, err := strconv.Atoi(v.Value)
			if err != nil || value < 1 || value > 65535 {
				return p, errors.New("invalid SMTP port")
			}
			p.Port = value
			continue
		}
		if key == "chat_id" && v.Kind == yaml.ScalarNode && v.Tag == "!!int" {
			p.ChatID = v.Value
			continue
		}
		if v.Kind != yaml.ScalarNode || v.Tag != "!!str" {
			return p, errors.New("invalid notification profile field")
		}
		switch key {
		case "name":
			p.Name = v.Value
		case "action":
			p.Action = v.Value
		case "chat_id":
			p.ChatID = v.Value
		case "token_env":
			p.TokenEnv = v.Value
		case "url_env":
			p.URLEnv = v.Value
		case "bearer_env":
			p.BearerEnv = v.Value
		case "allowed_origin":
			p.AllowedOrigin = v.Value
		case "host":
			p.Host = v.Value
		case "tls_mode":
			p.TLSMode = v.Value
		case "username_env":
			p.UsernameEnv = v.Value
		case "password_env":
			p.PasswordEnv = v.Value
		case "from":
			p.From = v.Value
		default:
			return p, errors.New("unknown notification profile field")
		}
	}
	return p, nil
}

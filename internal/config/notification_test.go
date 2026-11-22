package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const notificationYAML = `actions:
  notification:
    enabled: true
    profiles:
      - name: ops
        action: notification.webhook
        allowed_origin: https://example.com
        url_env: OPSWEAVE_ACTION_SECRET_URL
        bearer_env: OPSWEAVE_ACTION_SECRET_BEARER
`

func TestNotificationConfigReferencesAndStrictEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opsweave.yaml")
	if err := os.WriteFile(path, []byte(notificationYAML), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(t.Context(), path, dir, []string{"OPSWEAVE_ACTION_SECRET_URL=https://example.com/synthetic-secret", "OPSWEAVE_ACTION_SECRET_BEARER=synthetic-secret", "OPSWEAVE_NOTIFICATION_ENABLED=false"})
	if err != nil || cfg.Notification.Enabled || len(cfg.Notification.Profiles) != 1 {
		t.Fatalf("%+v %v", cfg.Notification, err)
	}
	data, err := json.Marshal(cfg)
	if err != nil || strings.Contains(string(data), "synthetic-secret") {
		t.Fatal("resolved credentials in configuration")
	}
	for _, env := range []string{"OPSWEAVE_ACTION_SECRET_UNUSED=synthetic-secret", "OPSWEAVE_NOTIFICATION_ENABLED=TRUE", "OPSWEAVE_NOTIFICATION_ENABLED="} {
		if _, err := Load(t.Context(), path, dir, []string{env}); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("unknown or invalid environment accepted")
		}
	}
	// Config validation is nonsecret and usable for history after credential removal.
	if _, err := Load(t.Context(), path, dir, nil); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationConfigYAMLValidation(t *testing.T) {
	for _, source := range []string{
		"actions: {notification: []}", "actions: {notification: {enabled: yes}}", "actions: {notification: {unknown: synthetic-secret}}", "actions: {notification: {profiles: {}}}",
		"actions: {notification: {profiles: [true]}}",
		strings.Replace(notificationYAML, "url_env:", "url:", 1), strings.Replace(notificationYAML, "name: ops", "name: true", 1),
		strings.Replace(notificationYAML, "allowed_origin: https://example.com", "allowed_origin: http://example.com", 1),
		strings.Replace(notificationYAML, "bearer_env: OPSWEAVE_ACTION_SECRET_BEARER", "recipients: [true]", 1),
		strings.Replace(notificationYAML, "bearer_env: OPSWEAVE_ACTION_SECRET_BEARER", "allowed_private_cidrs: [127.0.0.1/32, true]", 1),
		strings.Replace(notificationYAML, "bearer_env: OPSWEAVE_ACTION_SECRET_BEARER", "port: '465'", 1),
		strings.Replace(notificationYAML, "bearer_env: OPSWEAVE_ACTION_SECRET_BEARER", "port: 65536", 1),
	} {
		t.Run(source[:min(40, len(source))], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "opsweave.yaml")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(t.Context(), path, dir, nil)
			if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("unsafe notification configuration accepted")
			}
		})
	}
	for _, source := range []string{
		"actions: {notification: {enabled: true, profiles: [{name: chat, action: notification.telegram, chat_id: -123, token_env: OPSWEAVE_ACTION_SECRET_TOKEN}]}}",
		"actions: {notification: {enabled: true, profiles: [{name: chat, action: notification.slack, url_env: OPSWEAVE_ACTION_SECRET_URL}]}}",
		"actions: {notification: {enabled: true, profiles: [{name: mail, action: notification.email, host: smtp.example.com, port: 465, tls_mode: implicit_tls, username_env: OPSWEAVE_ACTION_SECRET_USER, password_env: OPSWEAVE_ACTION_SECRET_PASS, from: from@example.com, recipients: [to@example.com]}]}}",
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "opsweave.yaml")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(t.Context(), path, dir, nil); err != nil {
			t.Fatal(err)
		}
	}
}

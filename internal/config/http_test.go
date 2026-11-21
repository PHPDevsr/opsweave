package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("actions:\n  http:\n    enabled: true\n    allowed_origins: [https://example.com]\n    allowed_private_cidrs: [127.0.0.1/32]\n")
	c, err := Load(t.Context(), path, dir, nil)
	if err != nil || !c.HTTP.Enabled || len(c.HTTP.AllowedOrigins) != 1 {
		t.Fatal(c, err)
	}
	c, err = Load(t.Context(), path, dir, []string{"OPSWEAVE_HTTP_ENABLED=false", "OPSWEAVE_HTTP_ALLOWED_ORIGINS=https://other.example,http://localhost:80", "OPSWEAVE_HTTP_ALLOWED_PRIVATE_CIDRS="})
	if err != nil || c.HTTP.Enabled || len(c.HTTP.AllowedOrigins) != 2 || len(c.HTTP.AllowedPrivateCIDRs) != 0 {
		t.Fatal(c, err)
	}
	for _, s := range []string{
		"actions: {other: {}}", "actions: []", "actions: {http: []}",
		"actions: {http: {enabled: yes}}", "actions: {http: {enabled: true}}",
		"actions: {http: {allowed_origins: secret-marker}}",
		"actions: {http: {allowed_origins: [42]}}",
		"actions: {http: {allowed_private_cidrs: [0.0.0.0/0]}}",
		"actions: {http: {unknown: secret-marker}}",
		"actions: {http: {enabled: false, allowed_origins: [http://user:secret-marker@host]}}",
	} {
		write(s)
		if _, err := Load(t.Context(), path, dir, nil); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatalf("unsafe configuration error: %v", err)
		}
	}
	write("actions: {http: {}}")
	if _, err := Load(t.Context(), path, dir, []string{"OPSWEAVE_HTTP_ENABLED=yes"}); err == nil {
		t.Fatal("invalid boolean accepted")
	}
}

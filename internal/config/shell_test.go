package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	profile := fmt.Sprintf("{name: fixture, executable: %q, working_directory: %q, args: [echo]}", self, dir)
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("actions: {http: {enabled: true, allowed_origins: [https://example.com]}, shell: {enabled: true, commands: [" + profile + "]}}")
	c, err := Load(t.Context(), path, dir, nil)
	if err != nil || !c.Shell.Enabled || len(c.Shell.Commands) != 1 || !c.HTTP.Enabled {
		t.Fatal(c, err)
	}
	c, err = Load(t.Context(), path, dir, []string{"OPSWEAVE_SHELL_ENABLED=false"})
	if err != nil || c.Shell.Enabled || len(c.Shell.Commands) != 1 || !c.HTTP.Enabled {
		t.Fatal(c, err)
	}
	for _, text := range []string{
		"actions: {shell: []}", "actions: {shell: {enabled: yes}}", "actions: {shell: {enabled: true}}",
		"actions: {shell: {commands: [secret-marker]}}", "actions: {shell: {commands: {name: secret-marker}}}",
		"actions: {shell: {commands: [{name: secret-marker, executable: relative, working_directory: relative}]}}",
		"actions: {shell: {commands: [{name: fixture, executable: 42}]}}",
		"actions: {shell: {commands: [{name: fixture, args: [true]}]}}",
		"actions: {shell: {commands: [{name: fixture, args: secret-marker}]}}",
		"actions: {shell: {commands: [{unknown: secret-marker}]}}",
		"actions: {shell: {unknown: secret-marker}}",
		"actions: {shell: {commands: [" + profile + "," + profile + "]}}",
	} {
		write(text)
		if _, err := Load(t.Context(), path, dir, nil); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal("unsafe or missing error", err)
		}
	}
	write("actions: {shell: {}}")
	if _, err := Load(t.Context(), path, dir, []string{"OPSWEAVE_SHELL_ENABLED=yes"}); err == nil {
		t.Fatal("invalid environment setting accepted")
	}
	write("actions: {shell: {enabled: false, commands: [" + profile + "]}}")
	c, err = Load(t.Context(), path, dir, []string{"OPSWEAVE_SHELL_ENABLED=true"})
	if err != nil || !c.Shell.Enabled {
		t.Fatal(c, err)
	}
}

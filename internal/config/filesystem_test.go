package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemConfiguration(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	root := fmt.Sprintf("{name: root, path: %q, allowed_actions: [file.exists,file.copy]}", dir)
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("actions: {filesystem: {enabled: true, max_copy_bytes: 1048576, roots: [" + root + "]}}")
	c, err := Load(t.Context(), file, dir, nil)
	if err != nil || !c.Filesystem.Enabled || c.Filesystem.MaxCopyBytes != 1<<20 || len(c.Filesystem.Roots) != 1 || c.Filesystem.Roots[0].Path != dir {
		t.Fatal(c, err)
	}
	if c, err = Load(t.Context(), file, dir, []string{"OPSWEAVE_FILESYSTEM_ENABLED=false"}); err != nil || c.Filesystem.Enabled || len(c.Filesystem.Roots) != 1 {
		t.Fatal(c, err)
	}
	write("actions: {filesystem: {enabled: false, roots: [" + root + "]}}")
	if c, err = Load(t.Context(), file, dir, []string{"OPSWEAVE_FILESYSTEM_ENABLED=true"}); err != nil || !c.Filesystem.Enabled {
		t.Fatal(c, err)
	}
	for _, text := range []string{
		"actions: {filesystem: []}", "actions: {filesystem: {enabled: yes}}", "actions: {filesystem: {enabled: true}}",
		"actions: {filesystem: {max_copy_bytes: '1048576'}}", "actions: {filesystem: {max_copy_bytes: 0}}", "actions: {filesystem: {max_copy_bytes: 67108865}}",
		"actions: {filesystem: {roots: 12}}", "actions: {filesystem: {roots: [12]}}",
		"actions: {filesystem: {roots: [{name: root, path: secret-marker, allowed_actions: [file.exists]}]}}",
		"actions: {filesystem: {roots: [{name: root, path: 12}]}}",
		"actions: {filesystem: {roots: [{name: root, allowed_actions: 12}]}}",
		"actions: {filesystem: {roots: [{name: root, allowed_actions: [12]}]}}",
		"actions: {filesystem: {roots: [{unknown: secret-marker}]}}",
		"actions: {filesystem: {unknown: secret-marker}}",
		"actions: {filesystem: {roots: [" + root + "," + root + "]}}",
		"actions: {filesystem: {enabled: false, enabled: true}}",
	} {
		write(text)
		if _, err := Load(t.Context(), file, dir, nil); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal(text, err)
		}
	}
	write("actions: {filesystem: {}}")
	for _, value := range []string{"yes", "TRUE", "1", ""} {
		if _, err := Load(t.Context(), file, dir, []string{"OPSWEAVE_FILESYSTEM_ENABLED=" + value}); err == nil {
			t.Fatal("invalid override accepted", value)
		}
	}
	if _, err := Load(t.Context(), file, dir, []string{"OPSWEAVE_FILESYSTEM_ROOT=secret-marker"}); err == nil || strings.Contains(err.Error(), "secret-marker") {
		t.Fatal(err)
	}
}

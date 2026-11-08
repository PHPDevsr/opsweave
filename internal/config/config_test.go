package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/yamlutil"
)

func TestLoadDefaultsAndPrecedence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, err := Load(t.Context(), "", dir, nil)
	if err != nil || c.StoragePath != filepath.Join(dir, "data", "opsweave.db") || c.LogLevel != "info" {
		t.Fatalf("defaults = %+v, %v", c, err)
	}
	file := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(file, []byte("storage: {path: file.db}\nlogging: {level: warn}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = Load(t.Context(), file, dir, nil)
	if err != nil || c.StoragePath != filepath.Join(dir, "file.db") || c.LogLevel != "warn" {
		t.Fatalf("file = %+v, %v", c, err)
	}
	c, err = Load(t.Context(), file, dir, []string{"OPSWEAVE_STORAGE_PATH=env.db", "OPSWEAVE_LOG_LEVEL=debug", "UNRELATED=ok"})
	if err != nil || c.StoragePath != filepath.Join(dir, "env.db") || c.LogLevel != "debug" {
		t.Fatalf("environment = %+v, %v", c, err)
	}
	if _, err := Load(t.Context(), "missing.yaml", dir, nil); err == nil {
		t.Fatal("missing explicit config accepted")
	}
}

func TestConfigRejectsUnsafeInput(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"", "[]", "unknown: secret-marker", "storage: []", "storage: {path: true}", "logging: {other: secret-marker}",
		"storage: {path: ''}", "logging: {level: ''}", "logging: {level: trace}", "storage: {path: 'file:secret-marker'}",
		"logging: {level: null}", "logging: {level: info, level: warn}", "logging: &anchor {level: info}",
		"storage: {path: !!str secret-marker}", "{}\n---\n{}", "[secret-marker", strings.Repeat("#", (64<<10)+1), string([]byte{0xff}),
	} {
		t.Run(source[:min(len(source), 32)], func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "opsweave.yaml"), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(t.Context(), "", dir, nil)
			if err == nil || strings.Contains(err.Error(), "secret-marker") {
				t.Fatalf("unsafe or missing error: %v", err)
			}
		})
	}
	for _, env := range []string{"OPSWEAVE_UNKNOWN=secret-marker", "OPSWEAVE_LOG_LEVEL=", "OPSWEAVE_STORAGE_PATH=", "OPSWEAVE_LOG_LEVEL=secret-marker"} {
		if _, err := Load(t.Context(), "", t.TempDir(), []string{env}); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatalf("unsafe or missing error: %v", err)
		}
	}
}

func TestInitExclusiveAndCanceled(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := Init(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if err := Init(t.Context(), path); err == nil {
		t.Fatal("overwrote existing config")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != DefaultYAML {
		t.Fatalf("config = %q, %v", data, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Init(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Load(ctx, "", t.TempDir(), nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestInitRefusesSymlinkAndConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(t.Context(), dir, dir, nil); err == nil {
		t.Fatal("directory accepted as configuration file")
	}
	target := filepath.Join(dir, "existing.yaml")
	if err := Init(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows does not permit test symlink creation: %v", err)
		}
		t.Fatalf("create test symlink: %v", err)
	}
	if err := Init(t.Context(), link); err == nil {
		t.Fatal("init overwrote a symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != DefaultYAML {
		t.Fatal("symlink target changed", err)
	}
}

func TestStoragePathValidation(t *testing.T) {
	for _, path := range []string{"", "file:test.db", ":memory:", "https://secret-marker", "db?mode=memory", "//server/share/db", `\\server\share\db`, "db\x00tail"} {
		if ValidStoragePath(path) {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
	if !ValidStoragePath(filepath.Join(t.TempDir(), "space# & dir", "db.sqlite")) {
		t.Fatal("local path rejected")
	}
}

func FuzzConfig(f *testing.F) {
	for _, source := range []string{DefaultYAML, "[]", "storage: &s {path: test.db}", "{}"} {
		f.Add([]byte(source))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		node, err := yamlutil.Load(t.Context(), source, 64<<10)
		if err == nil {
			c := Config{StoragePath: "./data/opsweave.db", LogLevel: "info"}
			err = decode(node, &c)
		}
		if err != nil && len(err.Error()) > 200 {
			t.Fatal("unbounded diagnostic")
		}
	})
}

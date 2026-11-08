package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

const cliFixture = "# original comment\napiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - id: first\n      uses: test.echo\n      with: {message: hello}\n"

func testCatalog() registry.Catalog {
	return registry.Catalog{"test.echo": {ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"message": {Type: "string", Required: true}}}}}
}

func setupFoundation(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "opsweave.yaml")
	if err := config.Init(t.Context(), file); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "data", "opsweave.db")
	t.Setenv("OPSWEAVE_STORAGE_PATH", db)
	t.Setenv("OPSWEAVE_LOG_LEVEL", "info")
	input := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(input, []byte(cliFixture), 0600); err != nil {
		t.Fatal(err)
	}
	return file, input, db
}

func runCatalog(t *testing.T, catalog registry.Catalog, cfg string, args ...string) (string, error) {
	t.Helper()
	var out, diag bytes.Buffer
	argv := append([]string{"opsweave", "--config", cfg}, args...)
	err := executeWithCatalog(t.Context(), argv, &out, &diag, catalog)
	if diag.Len() > 0 {
		t.Fatalf("unexpected library diagnostic: %s", &diag)
	}
	return out.String(), err
}

func TestFoundationPreflightHasNoStorageSideEffects(t *testing.T) {
	cfg, input, db := setupFoundation(t)
	for _, verb := range []string{"validate", "plan"} {
		out, err := runCatalog(t, testCatalog(), cfg, verb, "--json", input)
		if err != nil || !json.Valid([]byte(out)) || !strings.Contains(out, `"order":["first"]`) {
			t.Fatalf("%s = %s, %v", verb, out, err)
		}
		if _, err := os.Stat(db); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("validation/planning created database")
		}
	}
	for _, verb := range []string{"validate", "plan", "apply"} {
		out, err := runCatalog(t, nil, cfg, verb, input)
		if err == nil || out != "" || !strings.Contains(err.Error(), "action unavailable") {
			t.Fatalf("empty production catalog = %s, %v", out, err)
		}
	}
	if _, err := os.Stat(db); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rejected deployment created database")
	}
	bad := filepath.Join(filepath.Dir(input), "bad.yaml")
	if err := os.WriteFile(bad, []byte("[invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", filepath.Dir(input)); err == nil {
		t.Fatal("invalid batch accepted")
	}
	if _, err := os.Stat(db); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid batch created database")
	}
}

func TestFoundationCLIEndToEnd(t *testing.T) {
	cfg, input, _ := setupFoundation(t)
	run := func(args ...string) string {
		t.Helper()
		out, err := runCatalog(t, testCatalog(), cfg, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	var records []registry.Record
	if err := json.Unmarshal([]byte(run("apply", "--json", input)), &records); err != nil || len(records) != 1 {
		t.Fatalf("apply records = %v, %v", records, err)
	}
	firstID := records[0].Revision
	if strings.Contains(run("workflow", "list"), "example") == false {
		t.Fatal("workflow not listed")
	}
	if out := run("workflow", "disable", "--json", "example"); !strings.Contains(out, `"enabled":false`) {
		t.Fatal(out)
	}
	run("apply", input)
	var record registry.Record
	if err := json.Unmarshal([]byte(run("workflow", "get", "--json", "example")), &record); err != nil || record.Number != 2 || record.Enabled {
		t.Fatalf("updated record = %+v, %v", record, err)
	}
	if out := run("workflow", "export", "--revision", firstID, "example"); out != cliFixture {
		t.Fatalf("export changed source: %q", out)
	}
	importPath := filepath.Join(t.TempDir(), "exported.yaml")
	if err := os.WriteFile(importPath, []byte(run("workflow", "export", "example")), 0600); err != nil {
		t.Fatal(err)
	}
	run("apply", importPath)
	if out := run("workflow", "get", "--revision", firstID, "example"); !strings.Contains(out, "Number: 1") {
		t.Fatal(out)
	}
	var history []registry.Record
	if err := json.Unmarshal([]byte(run("workflow", "revisions", "--json", "example")), &history); err != nil || len(history) != 3 {
		t.Fatalf("history = %v, %v", history, err)
	}
	run("workflow", "enable", "example")
	run("workflow", "delete", "example")
	if out := run("workflow", "list", "--json"); strings.TrimSpace(out) != "[]" {
		t.Fatal(out)
	}
	if out := run("workflow", "list", "--all", "--json"); !strings.Contains(out, `"archived":true`) {
		t.Fatal(out)
	}
	if _, err := runCatalog(t, testCatalog(), cfg, "workflow", "enable", "example"); !errors.Is(err, registry.ErrArchived) {
		t.Fatal(err)
	}
	if out := run("workflow", "export", "example"); out != cliFixture {
		t.Fatal("archive lost source")
	}
}

func TestFoundationInitHelpAndArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	var out, diag bytes.Buffer
	if code := Run(t.Context(), []string{"opsweave", "--config", path, "init", "--json"}, &out, &diag); code != 0 || strings.TrimSpace(out.String()) != `{"status":"created"}` {
		t.Fatalf("init = %d, %s, %s", code, &out, &diag)
	}
	out.Reset()
	diag.Reset()
	if code := Run(t.Context(), []string{"opsweave", "--config", path, "init"}, &out, &diag); code != 1 || out.Len() != 0 || strings.Count(diag.String(), "\n") != 1 {
		t.Fatalf("exclusive init = %d, %s, %s", code, &out, &diag)
	}
	cfg, _, _ := setupFoundation(t)
	for _, args := range [][]string{
		{"init", "extra"}, {"validate"}, {"plan", "a", "b"}, {"apply", "--unknown"},
		{"workflow", "list", "extra"}, {"workflow", "get"}, {"workflow", "export", "--json", "example"},
		{"workflow", "delete"}, {"workflow", "unknown"}, {"workflow", "enable", "a", "b"},
		{"workflow", "get", "--revision", "", "example"},
	} {
		if _, err := runCatalog(t, nil, cfg, args...); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
	for _, args := range [][]string{{"--help"}, {"version"}, {"completion", "bash"}, {"validate", "--help"}} {
		var stdout, stderr bytes.Buffer
		argv := append([]string{"opsweave", "--config", filepath.Join(t.TempDir(), "missing")}, args...)
		if err := execute(t.Context(), argv, &stdout, &stderr); err != nil {
			t.Fatalf("help/version/completion loaded configuration: %v", err)
		}
	}
}

func TestFoundationOutputFailureAfterCommit(t *testing.T) {
	cfg, input, db := setupFoundation(t)
	writeErr := errors.New("output unavailable")
	err := executeWithCatalog(t.Context(), []string{"opsweave", "--config", cfg, "apply", "--json", input}, failingWriter{writeErr}, io.Discard, testCatalog())
	if !errors.Is(err, writeErr) {
		t.Fatalf("writer error lost: %v", err)
	}
	store, err := sqlite.Open(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := store.Get(t.Context(), "example", ""); err != nil {
		t.Fatal("writer failure incorrectly rolled back committed data", err)
	}
	for _, args := range [][]string{{"workflow", "export", "example"}, {"workflow", "list"}, {"workflow", "get", "example"}} {
		argv := append([]string{"opsweave", "--config", cfg}, args...)
		if err := executeWithCatalog(t.Context(), argv, failingWriter{writeErr}, io.Discard, testCatalog()); !errors.Is(err, writeErr) {
			t.Fatalf("writer error lost for %v: %v", args, err)
		}
	}
}

func TestDebugDiagnosticsAndEmptyConfig(t *testing.T) {
	cfg, input, db := setupFoundation(t)
	t.Setenv("OPSWEAVE_LOG_LEVEL", "debug")
	var out, diag bytes.Buffer
	err := executeWithCatalog(t.Context(), []string{"opsweave", "--config", cfg, "validate", input}, &out, &diag, testCatalog())
	if err != nil || !json.Valid(diag.Bytes()) || !strings.Contains(diag.String(), `"command":"validate"`) || strings.Contains(diag.String(), db) {
		t.Fatalf("debug diagnostic = %s, %v", &diag, err)
	}
	writeErr := errors.New("diagnostic unavailable")
	err = executeWithCatalog(t.Context(), []string{"opsweave", "--config", cfg, "apply", input}, io.Discard, failingWriter{writeErr}, testCatalog())
	if !errors.Is(err, writeErr) {
		t.Fatal(err)
	}
	err = executeWithCatalog(t.Context(), []string{"opsweave", "--config", cfg, "apply", input}, io.Discard, failingWriter{}, testCatalog())
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short debug write was ignored: %v", err)
	}
	if _, err := os.Stat(db); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed diagnostic was ignored before database creation")
	}
	for _, args := range [][]string{{"init"}, {"workflow", "list"}, {"validate", input}} {
		argv := append([]string{"opsweave", "--config", ""}, args...)
		if err := execute(t.Context(), argv, io.Discard, io.Discard); err == nil {
			t.Fatal("empty explicit configuration flag accepted")
		}
	}
}

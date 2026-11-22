package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/dockeraction"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/fileaction"
	"github.com/PHPDevsr/opsweave/internal/httpaction"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/shellaction"
)

func filesystemPolicy(src, dst, grants string) string {
	return fmt.Sprintf("actions: {filesystem: {enabled: true, roots: [{name: source, path: %q, allowed_actions: [file.exists,file.copy]}, {name: destination, path: %q, allowed_actions: [%s]}]}}", src, dst, grants)
}

func TestConfiguredFilesystemPermissionsAreIndependent(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	endpoint := "unix:///tmp/opsweave-nonexistent.sock"
	if os.PathSeparator == '\\' {
		endpoint = "npipe:////./pipe/opsweave-nonexistent"
	}
	for _, httpOn := range []bool{false, true} {
		for _, shellOn := range []bool{false, true} {
			for _, dockerOn := range []bool{false, true} {
				for _, filesOn := range []bool{false, true} {
					cfg := config.Config{HTTP: httpaction.Config{Enabled: httpOn, AllowedOrigins: []string{"https://example.com"}}, Shell: shellaction.Config{Enabled: shellOn, Commands: []shellaction.Profile{{Name: "fixture", Executable: self, WorkingDirectory: dir}}}, Docker: dockeraction.Config{Enabled: dockerOn, Endpoint: endpoint, AllowedContainers: []string{dockerContainer}, AllowedActions: []string{"docker.inspect"}}, Filesystem: fileaction.Config{Enabled: filesOn, Roots: []fileaction.Root{{Name: "root", Path: dir, AllowedActions: []string{"file.exists"}}}}}
					r, catalog, err := configuredActions(cfg)
					if err != nil {
						t.Fatal(err)
					}
					want := 0
					if httpOn {
						want += 2
					}
					if shellOn {
						want++
					}
					if dockerOn {
						want++
					}
					if filesOn {
						want++
					}
					if len(catalog) != want {
						t.Fatal(catalog)
					}
					for id, enabled := range map[string]bool{"http.request": httpOn, "http.health_check": httpOn, "shell.exec": shellOn, "docker.inspect": dockerOn, "docker.start": false, "file.exists": filesOn, "file.copy": false, "file.cleanup": false} {
						err := r.Check(id, "1")
						if enabled && err != nil || !enabled && !errors.Is(err, execution.ErrUnavailable) {
							t.Fatal(id, err)
						}
					}
				}
			}
		}
	}
}

func TestProductionFilesystemWorkflowHistoryAndRevocation(t *testing.T) {
	cfg, input, _ := setupFoundation(t)
	src, dst := t.TempDir(), t.TempDir()
	content := "private-file-content-marker"
	if err := os.WriteFile(filepath.Join(src, "input"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(filesystemPolicy(src, dst, "file.exists,file.copy,file.cleanup")), 0600); err != nil {
		t.Fatal(err)
	}
	doc := `apiVersion: opsweave.io/v1
kind: Workflow
metadata: {name: example}
spec:
  steps:
    - {id: check, uses: file.exists, with: {root: source, path: input}}
    - {id: copy, uses: file.copy, with: {source_root: source, source: input, destination_root: destination, destination: copied}}
    - {id: confirm, uses: file.exists, with: {root: destination, path: copied}}
    - {id: remove, uses: file.cleanup, with: {root: destination, path: copied}}
    - {id: absent, uses: file.cleanup, with: {root: destination, path: copied}}
`
	if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"validate", "plan", "apply"} {
		if out, err := runCatalog(t, nil, cfg, verb, "--json", input); err != nil {
			t.Fatal(out, err)
		}
	}
	out, err := runCatalog(t, nil, cfg, "run", "--json", "example")
	if err != nil {
		t.Fatal(out, err)
	}
	var snap execution.Snapshot
	if err := json.Unmarshal([]byte(out), &snap); err != nil || snap.State != execution.Succeeded {
		t.Fatal(out, err)
	}
	id := snap.ID
	out, err = runCatalog(t, nil, cfg, "execution", "get", "--json", id)
	if err != nil || strings.Contains(out, content) || !strings.Contains(out, `"removed":false`) || !strings.Contains(out, `"removed":true`) || !strings.Contains(out, `"bytes":`) {
		t.Fatal(out, err)
	}
	if err := json.Unmarshal([]byte(out), &snap); err != nil || len(snap.Steps) != 5 {
		t.Fatal(out, err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "copied")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cleanup did not remove copied file", err)
	}
	if err := os.WriteFile(cfg, []byte(filesystemPolicy(src, dst, "file.exists,file.cleanup")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCatalog(t, nil, cfg, "run", "example"); !errors.Is(err, execution.ErrAction) {
		t.Fatal("destination grant not revoked", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "copied")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("revoked copy published file", err)
	}
	t.Setenv("OPSWEAVE_FILESYSTEM_ENABLED", "false")
	if _, err := runCatalog(t, nil, cfg, "apply", input); err == nil {
		t.Fatal("disabled filesystem action admitted")
	}
	if out, err := runCatalog(t, nil, cfg, "execution", "get", "--json", id); err != nil || !strings.Contains(out, `"removed":true`) {
		t.Fatal(out, err)
	}
}

func TestQueuedFilesystemShutdownPreservesCommittedCopy(t *testing.T) {
	cfg, input, _ := setupFoundation(t)
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "input"), []byte("committed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(filesystemPolicy(src, dst, "file.copy,file.cleanup")), 0600); err != nil {
		t.Fatal(err)
	}
	policy, err := config.Load(t.Context(), cfg, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fileaction.Registrations(policy.Filesystem)
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	// An injected test gate makes shutdown deterministic after a real committed
	// copy and before cleanup. It is never available through production config.
	entries = append(entries, action.Registration{Descriptor: registry.Descriptor{ID: "test.gate", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{}}}, Run: func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		close(ready)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	var grants []string
	bindings := map[string]string{}
	for _, entry := range entries {
		grants = append(grants, entry.Descriptor.Permissions...)
		bindings[entry.Descriptor.ID] = entry.Descriptor.Version
	}
	r, err := action.New(entries, grants)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := r.Catalog(bindings)
	if err != nil {
		t.Fatal(err)
	}
	doc := `apiVersion: opsweave.io/v1
kind: Workflow
metadata: {name: example}
spec:
  steps:
    - {id: copy, uses: file.copy, with: {source_root: source, source: input, destination_root: destination, destination: copied}}
    - {id: wait, uses: test.gate}
    - {id: remove, uses: file.cleanup, with: {root: destination, path: copied}}
`
	if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (string, error) {
		t.Helper()
		var out strings.Builder
		err := executeWithRuntime(t.Context(), append([]string{"opsweave", "--config", cfg}, args...), &out, io.Discard, catalog, r)
		return out.String(), err
	}
	if out, err := invoke("apply", input); err != nil {
		t.Fatal(out, err)
	}
	out, err := invoke("run", "--enqueue", "--json", "example")
	if err != nil {
		t.Fatal(out, err)
	}
	var snap execution.Snapshot
	if err := json.Unmarshal([]byte(out), &snap); err != nil || snap.State != execution.Queued {
		t.Fatal(out, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- executeWithRuntime(ctx, []string{"opsweave", "--config", cfg, "server"}, io.Discard, io.Discard, catalog, r)
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("gate never reached after copy", ctx.Err())
	}
	if data, err := os.ReadFile(filepath.Join(dst, "copied")); err != nil || string(data) != "committed" {
		t.Fatal(data, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("queued server did not join")
	}
	out, err = invoke("execution", "get", "--json", snap.ID)
	if err != nil {
		t.Fatal(out, err)
	}
	if err := json.Unmarshal([]byte(out), &snap); err != nil || snap.State != execution.Cancelled || len(snap.Steps) != 3 || snap.Steps[0].State != execution.Succeeded || len(snap.Steps[2].Attempts) != 0 {
		t.Fatal(out, err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "copied")); err != nil || string(data) != "committed" {
		t.Fatal("shutdown rolled back copy or ran pending cleanup", data, err)
	}
}

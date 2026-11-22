package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/dockeraction"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/httpaction"
	"github.com/PHPDevsr/opsweave/internal/shellaction"
	"github.com/PHPDevsr/opsweave/internal/testutil/localipc"
)

const dockerContainer = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func dockerPolicy(endpoint string, actions string) string {
	return fmt.Sprintf("actions: {docker: {enabled: true, endpoint: %q, allowed_containers: [%q], allowed_actions: [%s]}}", endpoint, dockerContainer, actions)
}

func TestConfiguredDockerPermissionsAreIndependent(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, httpEnabled := range []bool{false, true} {
		for _, shellEnabled := range []bool{false, true} {
			for _, dockerEnabled := range []bool{false, true} {
				cfg := config.Config{HTTP: httpaction.Config{Enabled: httpEnabled, AllowedOrigins: []string{"https://example.com"}}, Shell: shellaction.Config{Enabled: shellEnabled, Commands: []shellaction.Profile{{Name: "fixture", Executable: self, WorkingDirectory: t.TempDir()}}}, Docker: dockeraction.Config{Enabled: dockerEnabled, AllowedContainers: []string{dockerContainer}, AllowedActions: []string{"docker.inspect"}}}
				// An endpoint that does not exist proves composition performs no I/O.
				if os.PathSeparator == '\\' {
					cfg.Docker.Endpoint = "npipe:////./pipe/opsweave-nonexistent"
				} else {
					cfg.Docker.Endpoint = "unix:///tmp/opsweave-nonexistent.sock"
				}
				r, catalog, err := configuredActions(cfg)
				if err != nil {
					t.Fatal(err)
				}
				want := 0
				if httpEnabled {
					want += 2
				}
				if shellEnabled {
					want++
				}
				if dockerEnabled {
					want++
				}
				if len(catalog) != want {
					t.Fatal(catalog)
				}
				for _, id := range []string{"http.request", "http.health_check", "shell.exec", "docker.inspect", "docker.start", "docker.stop", "docker.restart"} {
					enabled := httpEnabled
					if id == "shell.exec" {
						enabled = shellEnabled
					} else if strings.HasPrefix(id, "docker.") {
						enabled = dockerEnabled && id == "docker.inspect"
					}
					err := r.Check(id, "1")
					if enabled && err != nil || !enabled && !errors.Is(err, execution.ErrUnavailable) {
						t.Fatal(id, err)
					}
				}
			}
		}
	}
}

func TestProductionDockerWorkflowHistoryAndRevocation(t *testing.T) {
	var requests atomic.Int32
	endpoint := localipc.Serve(t, func(conn net.Conn) {
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			t.Error(err)
			return
		}
		requests.Add(1)
		if err := req.Body.Close(); err != nil {
			t.Error(err)
		}
		if strings.HasSuffix(req.URL.Path, "/json") {
			body := fmt.Sprintf(`{"Id":%q,"Name":"/fixture","Image":"sha256:%s","State":{"Status":"running","Running":true,"ExitCode":0},"Config":{"Env":["TOKEN=secret-marker"]}}`, dockerContainer, strings.Repeat("b", 64))
			if _, err := fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body); err != nil {
				t.Error(err)
			}
		} else {
			if _, err := io.WriteString(conn, "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n"); err != nil {
				t.Error(err)
			}
		}
	})
	cfg, input, _ := setupFoundation(t)
	if err := os.WriteFile(cfg, []byte(dockerPolicy(endpoint, "docker.start,docker.inspect,docker.stop,docker.restart")), 0600); err != nil {
		t.Fatal(err)
	}
	doc := "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n"
	for i, id := range []string{"start", "inspect", "stop", "restart"} {
		doc += fmt.Sprintf("    - {id: step%d, uses: docker.%s, with: {container: %q}}\n", i, id, dockerContainer)
	}
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
	if err := json.Unmarshal([]byte(out), &snap); err != nil || snap.State != execution.Succeeded || requests.Load() != 4 {
		t.Fatal(out, requests.Load(), err)
	}
	out, err = runCatalog(t, nil, cfg, "execution", "get", "--json", snap.ID)
	if err != nil || strings.Contains(out, "secret-marker") || !strings.Contains(out, `"image_id"`) || !strings.Contains(out, `"changed":true`) {
		t.Fatal(out, err)
	}
	if err := json.Unmarshal([]byte(out), &snap); err != nil || len(snap.Steps) != 4 {
		t.Fatal("durable history lost steps", out, err)
	}
	if err := os.WriteFile(cfg, []byte(dockerPolicy(endpoint, "docker.inspect")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCatalog(t, nil, cfg, "run", "example"); err == nil {
		t.Fatal("revoked mutation permission invoked")
	}
	if requests.Load() != 4 {
		t.Fatal("revocation contacted daemon")
	}
	t.Setenv("OPSWEAVE_DOCKER_ENABLED", "false")
	if _, err := runCatalog(t, nil, cfg, "apply", input); err == nil {
		t.Fatal("disabled action admitted")
	}
	if out, err := runCatalog(t, nil, cfg, "execution", "get", "--json", snap.ID); err != nil || !strings.Contains(out, `"image_id"`) {
		t.Fatal(out, err)
	}
}

func TestProductionQueuedDockerShutdown(t *testing.T) {
	ready := make(chan struct{})
	peerClosed := make(chan struct{})
	endpoint := localipc.Serve(t, func(conn net.Conn) {
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			t.Error(err)
			return
		}
		close(ready)
		_, _ = io.Copy(io.Discard, conn)
		close(peerClosed)
	})
	cfg, input, _ := setupFoundation(t)
	if err := os.WriteFile(cfg, []byte(dockerPolicy(endpoint, "docker.inspect")), 0600); err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - {id: first, uses: docker.inspect, with: {container: %q}}\n", dockerContainer)
	if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := runCatalog(t, nil, cfg, "apply", input); err != nil {
		t.Fatal(out, err)
	}
	out, err := runCatalog(t, nil, cfg, "run", "--enqueue", "--json", "example")
	if err != nil {
		t.Fatal(out, err)
	}
	var snap execution.Snapshot
	if err := json.Unmarshal([]byte(out), &snap); err != nil || snap.State != execution.Queued {
		t.Fatal(out, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- execute(ctx, []string{"opsweave", "--config", cfg, "server"}, io.Discard, io.Discard)
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("Docker request never started", ctx.Err())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("server did not join Docker action")
	}
	select {
	case <-peerClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("daemon IPC survived shutdown")
	}
	out, err = runCatalog(t, nil, cfg, "execution", "get", "--json", snap.ID)
	if err != nil || !strings.Contains(out, `"state":"cancelled"`) || strings.Contains(out, `"container_id"`) {
		t.Fatal(out, err)
	}
}

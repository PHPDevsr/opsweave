package cli

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/notificationaction"
)

func TestConfiguredNotificationOptInAndSnapshot(t *testing.T) {
	const reference = "OPSWEAVE_ACTION_SECRET_CLI_URL"
	t.Setenv(reference, "https://example.com/synthetic-secret")
	cfg := config.Config{Notification: notificationaction.Config{Enabled: true, Profiles: []notificationaction.Profile{{Name: "ops", Action: "notification.webhook", AllowedOrigin: "https://example.com", URLEnv: reference}}}}
	runtime, catalog, err := configuredActions(cfg)
	if err != nil || len(catalog) != 1 {
		t.Fatal(err)
	}
	t.Setenv(reference, "")
	if runtime.Check("notification.webhook", "1") != nil {
		t.Fatal("credential snapshot disappeared")
	}
	for _, id := range []string{"notification.email", "notification.slack", "notification.telegram", "http.request", "shell.exec", "docker.inspect", "file.copy"} {
		if !errors.Is(runtime.Check(id, "1"), execution.ErrUnavailable) {
			t.Fatal("unconfigured action available", id)
		}
	}
	if _, _, err := configuredActions(cfg); err == nil {
		t.Fatal("missing credential accepted by new runtime")
	}
	cfg.Notification.Enabled = false
	if _, catalog, err := configuredActions(cfg); err != nil || len(catalog) != 0 {
		t.Fatal("disabled runtime read credential", err)
	}
}

func TestNotificationWorkflowAndQueuedShutdown(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(fmt.Sprint(queued), func(t *testing.T) {
			cfg, input, _ := setupFoundation(t)
			ready := make(chan struct{})
			released := make(chan struct{})
			var hits atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.Header.Get("Authorization") != "Bearer synthetic-bearer" || r.URL.Path != "/synthetic-url-secret" {
					t.Error("wrong credentials")
				}
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Error(err)
				}
				if queued {
					close(ready)
					<-r.Context().Done()
					close(released)
					return
				}
				w.WriteHeader(204)
			}))
			defer server.Close()
			policy := fmt.Sprintf("actions: {notification: {enabled: true, profiles: [{name: ops, action: notification.webhook, allowed_origin: %q, allowed_private_cidrs: [127.0.0.1/32], url_env: OPSWEAVE_ACTION_SECRET_CLI_URL, bearer_env: OPSWEAVE_ACTION_SECRET_CLI_BEARER}]}}", server.URL)
			if err := os.WriteFile(cfg, []byte(policy), 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := config.Load(t.Context(), cfg, t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			pool := x509.NewCertPool()
			pool.AddCert(server.Certificate())
			entries, err := notificationaction.Registrations(loaded.Notification, notificationaction.Dependencies{RootCAs: pool, LookupEnv: func(name string) (string, bool) {
				if strings.HasSuffix(name, "BEARER") {
					return "synthetic-bearer", true
				}
				return server.URL + "/synthetic-url-secret", true
			}})
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := action.New(entries, []string{"notification.webhook"})
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := runtime.Catalog(map[string]string{"notification.webhook": "1"})
			if err != nil {
				t.Fatal(err)
			}
			doc := "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - {id: notify, uses: notification.webhook, with: {profile: ops, message: ordinary-message}}\n"
			if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
				t.Fatal(err)
			}
			invoke := func(args ...string) (string, error) {
				t.Helper()
				var out, diag strings.Builder
				err := executeWithRuntime(t.Context(), append([]string{"opsweave", "--config", cfg}, args...), &out, &diag, catalog, runtime)
				combined := out.String() + diag.String()
				for _, marker := range []string{"synthetic-bearer", "synthetic-url-secret"} {
					if strings.Contains(combined, marker) {
						t.Fatal("credential in CLI output or logs")
					}
				}
				return out.String(), err
			}
			for _, verb := range []string{"validate", "plan", "apply"} {
				if out, err := invoke(verb, "--json", input); err != nil {
					t.Fatal(out, err)
				}
			}
			if hits.Load() != 0 {
				t.Fatal("preflight sent notification")
			}
			args := []string{"run", "--json", "example"}
			if queued {
				args = []string{"run", "--enqueue", "--json", "example"}
			}
			out, err := invoke(args...)
			if err != nil {
				t.Fatal(out, err)
			}
			var snap execution.Snapshot
			if err := json.Unmarshal([]byte(out), &snap); err != nil {
				t.Fatal(err)
			}
			if queued {
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				done := make(chan error, 1)
				go func() {
					done <- executeWithRuntime(ctx, []string{"opsweave", "--config", cfg, "server"}, io.Discard, io.Discard, catalog, runtime)
					close(done)
				}()
				t.Cleanup(func() { cancel(); <-done })
				select {
				case <-ready:
				case <-ctx.Done():
					t.Fatal("notification never started")
				}
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(8 * time.Second):
					t.Fatal("server failed to join notification")
				}
				select {
				case <-released:
				case <-time.After(3 * time.Second):
					t.Fatal("HTTPS peer leaked")
				}
			}
			out, err = invoke("execution", "get", "--json", snap.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(out), &snap); err != nil {
				t.Fatal(err)
			}
			want := execution.Succeeded
			if queued {
				want = execution.Cancelled
			}
			if snap.State != want || hits.Load() != 1 {
				t.Fatalf("state=%s hits=%d", snap.State, hits.Load())
			}
			if queued && strings.Contains(out, `"accepted":true`) || !queued && !strings.Contains(out, `"accepted":true`) {
				t.Fatal("incorrect durable output", out)
			}
			// History remains readable without credentials, using the production command composition.
			if err := os.WriteFile(cfg, []byte("actions: {notification: {enabled: false}}"), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := runCatalog(t, nil, cfg, "execution", "get", "--json", snap.ID); err != nil || !strings.Contains(out, snap.ID) {
				t.Fatal(out, err)
			}
			if _, err := runCatalog(t, nil, cfg, "run", "example"); err == nil {
				t.Fatal("revoked notification action ran")
			}
			if hits.Load() != 1 {
				t.Fatal("revocation sent notification")
			}
		})
	}
}

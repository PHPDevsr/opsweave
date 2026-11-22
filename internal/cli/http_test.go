package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

func TestProductionHTTPWorkflow(t *testing.T) {
	cfg, input, _ := setupFoundation(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, "ready") }))
	defer srv.Close()
	policy := fmt.Sprintf("actions:\n  http:\n    enabled: true\n    allowed_origins: [%q]\n    allowed_private_cidrs: [127.0.0.1/32]\n", srv.URL)
	if err := os.WriteFile(cfg, []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - id: first\n      uses: http.health_check\n      with: {url: %q}\n    - id: second\n      needs: [first]\n      uses: http.request\n      with: {url: %q}\n", srv.URL, srv.URL)
	if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (string, int, string) {
		t.Helper()
		var out, diag bytes.Buffer
		code := Run(t.Context(), append([]string{"opsweave", "--config", cfg}, args...), &out, &diag)
		return out.String(), code, diag.String()
	}
	for _, verb := range []string{"validate", "plan", "apply"} {
		out, code, err := invoke(verb, "--json", input)
		if code != 0 {
			t.Fatal(verb, out, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("preflight performed I/O")
	}
	out, code, diag := invoke("run", "--json", "example")
	if code != 0 {
		t.Fatal(out, diag)
	}
	var snap execution.Snapshot
	if err := json.Unmarshal([]byte(out), &snap); err != nil || snap.State != execution.Succeeded {
		t.Fatal(out, err)
	}
	out, code, diag = invoke("execution", "get", "--json", snap.ID)
	if code != 0 || !strings.Contains(out, "ready") || !strings.Contains(out, `"status_code":200`) {
		t.Fatal(out, diag)
	}
	if hits.Load() != 2 {
		t.Fatal(hits.Load())
	}
	// Removing the grant affects execution without changing the historical revision.
	if err := os.WriteFile(cfg, []byte("actions: {http: {enabled: false}}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, code, _ := invoke("apply", input); code == 0 {
		t.Fatal("disabled action accepted")
	}
	if _, code, _ := invoke("run", "example"); code == 0 {
		t.Fatal("disabled action invoked")
	}
	if hits.Load() != 2 {
		t.Fatal("disabled runtime reached endpoint")
	}
}

func TestProductionHTTPServerCancellation(t *testing.T) {
	cfg, input, _ := setupFoundation(t)
	entered := make(chan struct{})
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(released) }))
	defer srv.Close()
	policy := fmt.Sprintf("actions: {http: {enabled: true, allowed_origins: [%q], allowed_private_cidrs: [127.0.0.1/32]}}", srv.URL)
	if err := os.WriteFile(cfg, []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  triggers: [{id: boot, type: startup}]\n  steps:\n    - id: first\n      uses: http.request\n      with: {url: %q}\n", srv.URL)
	if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCatalog(t, nil, cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	done := make(chan error, 1)
	go func() { done <- execute(ctx, []string{"opsweave", "--config", cfg, "server"}, io.Discard, io.Discard) }()
	defer cancel()
	select {
	case <-entered:
	case err := <-done:
		t.Fatal("server ended early", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not join HTTP action")
	}
	select {
	case <-released:
	case <-time.After(3 * time.Second):
		t.Fatal("server left HTTP connection open")
	}
	out, err := runCatalog(t, nil, cfg, "execution", "list", "--json")
	if err != nil || !strings.Contains(out, `"state":"cancelled"`) {
		t.Fatal(out, err)
	}
}

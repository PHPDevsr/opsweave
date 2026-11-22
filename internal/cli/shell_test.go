package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/httpaction"
	"github.com/PHPDevsr/opsweave/internal/shellaction"
)

func TestConfiguredActionPermissionsAreIndependent(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, httpEnabled := range []bool{false, true} {
		for _, shellEnabled := range []bool{false, true} {
			cfg := config.Config{HTTP: httpaction.Config{Enabled: httpEnabled, AllowedOrigins: []string{"https://example.com"}}, Shell: shellaction.Config{Enabled: shellEnabled, Commands: []shellaction.Profile{{Name: "fixture", Executable: self, WorkingDirectory: t.TempDir()}}}}
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
			if len(catalog) != want {
				t.Fatal("incorrect catalog", catalog)
			}
			for _, id := range []string{"http.request", "http.health_check", "shell.exec"} {
				enabled := httpEnabled
				if id == "shell.exec" {
					enabled = shellEnabled
				}
				err := r.Check(id, "1")
				if enabled && err != nil || !enabled && !errors.Is(err, execution.ErrUnavailable) {
					t.Fatal(id, err)
				}
			}
		}
	}
}

func shellFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "../shellaction/testdata/fixture")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, output)
	}
	return path
}

func TestProductionShellWorkflowAndRevocation(t *testing.T) {
	cfg, input, _ := setupFoundation(t)
	program := shellFixture(t)
	dir := t.TempDir()
	policy := fmt.Sprintf("actions:\n  shell:\n    enabled: true\n    commands:\n      - name: fixture\n        executable: %q\n        working_directory: %q\n        args: [echo]\n", program, dir)
	if err := os.WriteFile(cfg, []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	doc := "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - id: first\n      uses: shell.exec\n      with:\n        command: fixture\n        args: ['literal; value']\n"
	if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		err := execute(t.Context(), append([]string{"opsweave", "--config", cfg}, args...), &out, io.Discard)
		return out.String(), err
	}
	for _, verb := range []string{"validate", "plan", "apply"} {
		if out, err := invoke(verb, "--json", input); err != nil {
			t.Fatal(out, err)
		}
	}
	out, err := invoke("run", "--json", "example")
	if err != nil {
		t.Fatal(out, err)
	}
	var snap execution.Snapshot
	if err := json.Unmarshal([]byte(out), &snap); err != nil || snap.State != execution.Succeeded {
		t.Fatal(out, err)
	}
	out, err = invoke("execution", "get", "--json", snap.ID)
	if err != nil || !strings.Contains(out, `"stdout":"literal; value"`) || !strings.Contains(out, `"exit_code":0`) {
		t.Fatal(out, err)
	}
	t.Setenv("OPSWEAVE_SHELL_ENABLED", "false")
	if _, err := invoke("apply", input); err == nil {
		t.Fatal("disabled action accepted")
	}
	if _, err := invoke("run", "example"); err == nil {
		t.Fatal("disabled action invoked")
	}
	// History survives revocation of the invocation permission.
	if out, err := invoke("execution", "get", "--json", snap.ID); err != nil || !strings.Contains(out, "literal; value") {
		t.Fatal(out, err)
	}
}

func TestProductionQueuedShellShutdown(t *testing.T) {
	cfg, input, _ := setupFoundation(t)
	program := shellFixture(t)
	dir := t.TempDir()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf("actions: {shell: {enabled: true, commands: [{name: fixture, executable: %q, working_directory: %q, args: [tree]}]}}", program, dir)
	if err := os.WriteFile(cfg, []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf("apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - id: first\n      uses: shell.exec\n      with: {command: fixture, args: [%q]}\n", listener.Addr().String())
	if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCatalog(t, nil, cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	out, err := runCatalog(t, nil, cfg, "run", "--enqueue", "--json", "example")
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
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("owned test goroutine did not join")
		}
	})
	go func() {
		defer close(done)
		done <- execute(ctx, []string{"opsweave", "--config", cfg, "server"}, io.Discard, io.Discard)
	}()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(8 * time.Second)); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	if ready, err := reader.ReadString('\n'); err != nil || strings.TrimSpace(ready) == "" {
		t.Fatal("child was not ready", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not join shell")
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	// Windows terminates sockets with WSAECONNRESET (10054) when killing a job.
	if _, err := io.Copy(io.Discard, reader); err != nil && !(runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10054))) {
		t.Fatal("descendant survived shutdown", err)
	}
	out, err = runCatalog(t, nil, cfg, "execution", "get", "--json", snap.ID)
	if err != nil || !strings.Contains(out, `"state":"cancelled"`) {
		t.Fatal(out, err)
	}
}

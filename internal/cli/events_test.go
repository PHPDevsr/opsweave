package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/event"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

func TestInternalEventCLI(t *testing.T) {
	cfg, path, _ := setupFoundation(t)
	doc := strings.Replace(cliFixture, "spec:", "spec:\n  triggers: [{id: build, type: internal, event: changed}]", 1)
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", path); err != nil {
		t.Fatal(err)
	}
	out, err := runCatalog(t, testCatalog(), cfg, "event", "emit", "--json", "--id", "stable", "changed")
	var receipts []event.Receipt
	if err != nil || json.Unmarshal([]byte(out), &receipts) != nil || len(receipts) != 1 || receipts[0].Trigger != "build" {
		t.Fatal(out, err)
	}
	out, err = runCatalog(t, testCatalog(), cfg, "event", "history", "--json", "example")
	if err != nil || !strings.Contains(out, receipts[0].Execution) || strings.Contains(out, "stable") {
		t.Fatal(out, err)
	}
}

func TestServerStartupTriggerExecutes(t *testing.T) {
	cfg, path, _ := setupFoundation(t)
	doc := strings.Replace(cliFixture, "spec:", "spec:\n  triggers: [{id: boot, type: startup}]", 1)
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", path); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	runtime, err := action.New([]action.Registration{{Descriptor: testCatalog()["test.echo"], Run: func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- executeWithRuntime(ctx, []string{"opsweave", "--config", cfg, "server"}, io.Discard, io.Discard, testCatalog(), runtime)
	}()
	select {
	case <-started:
	case err := <-finished:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("startup worker did not join")
	}
	out, err := runCatalog(t, testCatalog(), cfg, "event", "history", "--json", "example")
	if err != nil || !strings.Contains(out, `"type":"startup"`) {
		t.Fatal(out, err)
	}
}

func TestWebhookServerJoinsHandlerAndWorker(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered := make(chan struct{})
	handlerDone := make(chan struct{})
	workerDone := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(handlerDone) })
	result := make(chan error, 1)
	go func() {
		result <- runWebhookServer(ctx, io.Discard, listener, handler, func(ctx context.Context, _ string) (execution.Snapshot, error) {
			<-ctx.Done()
			close(workerDone)
			return execution.Snapshot{}, ctx.Err()
		})
	}()
	client := http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	requestDone := make(chan error, 1)
	go func() {
		resp, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			err = resp.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-entered:
	case err := <-requestDone:
		t.Fatalf("request ended before entering handler: %v", err)
	case <-time.After(4 * time.Second):
		t.Fatal("listener never entered handler")
	}
	cancel()
	select {
	case err := <-result:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("server did not shut down")
	}
	for _, done := range []chan struct{}{handlerDone, workerDone} {
		select {
		case <-done:
		default:
			t.Fatal("owned work survived shutdown")
		}
	}
	<-requestDone
}

func TestServerRejectsInvalidWebhookSettings(t *testing.T) {
	cfg, _, _ := setupFoundation(t)
	t.Setenv("OPSWEAVE_WEBHOOK_SECRET", "short")
	for _, test := range []struct{ port, diagnostic string }{{"0", "webhook port"}, {"8080", "OPSWEAVE_WEBHOOK_SECRET"}} {
		_, err := runCatalog(t, testCatalog(), cfg, "server", "--webhook-port", test.port)
		if err == nil || !strings.Contains(err.Error(), test.diagnostic) || strings.Contains(err.Error(), "short") {
			t.Fatal(err)
		}
	}
}

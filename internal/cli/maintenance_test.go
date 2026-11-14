package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/dispatchlock"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

func TestCLIPruneRequiresOptInAndExplicitMode(t *testing.T) {
	cfg, input, path := setupFoundation(t)
	for _, args := range [][]string{{"execution", "prune"}, {"execution", "prune", "--dry-run", "--apply"}, {"execution", "prune", "--apply"}} {
		if _, err := runCatalog(t, testCatalog(), cfg, args...); err == nil {
			t.Fatal("accepted without retention configuration and explicit mode", args)
		}
	}
	t.Setenv("OPSWEAVE_EXECUTION_RETENTION_AGE", "24h")
	if _, err := runCatalog(t, testCatalog(), cfg, "execution", "prune", "--dry-run"); err == nil {
		t.Fatal("missing database passed read-only preview")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preview created a database", err)
	}
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"--dry-run", "--apply"} {
		out, err := runCatalog(t, testCatalog(), cfg, "execution", "prune", mode, "--json")
		if err != nil || !strings.Contains(out, `"eligible":0`) || !strings.Contains(out, `"deleted":0`) {
			t.Fatal(out, err)
		}
	}
}

func TestServerStatusRequiresServerLocksAndLiveSchema(t *testing.T) {
	cfg, _, path := setupFoundation(t)
	assertStatus := func(want sqlite.ServerState) {
		t.Helper()
		out, err := runCatalog(t, testCatalog(), cfg, "server", "status", "--json")
		if strings.TrimSpace(out) != `{"state":"`+string(want)+`"}` || (err == nil) != (want == sqlite.ServerReady) {
			t.Fatalf("want %s, got %s (%v)", want, out, err)
		}
	}
	assertStatus(sqlite.ServerUnavailable)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("status created a database", err)
	}
	lock, err := dispatchlock.Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	})
	assertStatus(sqlite.ServerUnavailable)
	serverLock, err := lock.Serve()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := serverLock.Close(); err != nil {
			t.Error(err)
		}
	})
	assertStatus(sqlite.ServerUnavailable) // The database exists but has no schema.
	s, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	f, err := s.BeginDispatcher(t.Context(), strings.Repeat("a", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(sqlite.ServerUnavailable)
	stale := f
	stale.Generation++
	if err = s.PublishServerState(t.Context(), stale, sqlite.ServerReady); !errors.Is(err, execution.ErrConflict) {
		t.Fatal(err)
	}
	if err = s.PublishServerState(t.Context(), f, sqlite.ServerReady); err != nil {
		t.Fatal(err)
	}
	assertStatus(sqlite.ServerReady)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := localServerState(ctx, path); got != sqlite.ServerUnavailable || !errors.Is(err, context.Canceled) {
		t.Fatal(got, err)
	}
	if err = s.PublishServerState(t.Context(), f, sqlite.ServerStopping); err != nil {
		t.Fatal(err)
	}
	assertStatus(sqlite.ServerStopping)
	if err = serverLock.Close(); err != nil {
		t.Fatal(err)
	}
	assertStatus(sqlite.ServerUnavailable) // Stale SQL state and a standalone lock.
}

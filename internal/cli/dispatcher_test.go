package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

type readyWriter struct {
	once  sync.Once
	ready chan struct{}
}

func (w *readyWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "dispatcher ready") {
		w.once.Do(func() { close(w.ready) })
	}
	return len(p), nil
}

func TestDispatcherCLIServerQueueBusyAndShutdown(t *testing.T) {
	cfg, input, path := setupFoundation(t)
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	started := make(chan struct{}, 1)
	cancelling := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var calls atomic.Int32
	runtime, err := action.New([]action.Registration{{Descriptor: testCatalog()["test.echo"], Run: func(ctx context.Context, in map[string]any) (map[string]any, error) {
		calls.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		cancelling <- struct{}{}
		<-release
		return nil, ctx.Err()
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (execution.Snapshot, error) {
		var out bytes.Buffer
		err := executeWithRuntime(ctx, append([]string{"opsweave", "--config", cfg}, args...), &out, io.Discard, testCatalog(), runtime)
		var snap execution.Snapshot
		if out.Len() > 0 {
			if decodeErr := json.Unmarshal(out.Bytes(), &snap); decodeErr != nil {
				t.Fatal(decodeErr, out.String())
			}
		}
		return snap, err
	}
	queued, err := invoke("run", "--enqueue", "--json", "example")
	if err != nil || queued.State != execution.Queued || queued.Deadline != nil || calls.Load() != 0 {
		t.Fatal(queued, err)
	}
	serverCtx, stop := context.WithCancel(ctx)
	ready := &readyWriter{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- executeWithRuntime(serverCtx, []string{"opsweave", "--config", cfg, "server"}, ready, io.Discard, testCatalog(), runtime)
	}()
	select {
	case <-ready.ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if status, err := localServerStatus(ctx, path); err != nil || status.State != sqlite.ServerReady || status.Pool == nil || *status.Pool != (sqlite.WorkerPoolStatus{Capacity: 4, Busy: 1}) {
		t.Fatal(status, status.Pool, err)
	}
	if _, err = invoke("run", "--json", "example"); !errors.Is(err, execution.ErrBusy) {
		t.Fatal(err)
	}
	next, err := invoke("run", "--enqueue", "--json", "example")
	if err != nil || next.State != execution.Queued || next.ID == queued.ID {
		t.Fatal(next, err)
	}
	if status, err := localServerStatus(ctx, path); err != nil || status.Pool == nil || status.Pool.QueueDepth != 1 {
		t.Fatal(status, status.Pool, err)
	}
	stop()
	select {
	case <-cancelling:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for {
		status, err := localServerStatus(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == sqlite.ServerStopping {
			if status.Pool == nil || *status.Pool != (sqlite.WorkerPoolStatus{Capacity: 4, Busy: 1, QueueDepth: 1}) {
				t.Fatal(status, status.Pool)
			}
			break
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if status, err := localServerStatus(ctx, path); err != nil || status != (sqlite.ServerStatus{State: sqlite.ServerUnavailable}) {
		t.Fatal(status, err)
	}
	s, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	first, err := s.Execution(ctx, queued.ID)
	if err != nil || first.State != execution.Cancelled || calls.Load() != 1 {
		t.Fatal(first, err, calls.Load())
	}
	waiting, err := s.Execution(ctx, next.ID)
	if err != nil || waiting.State != execution.Queued {
		t.Fatal(waiting, err)
	}
}

func TestDispatcherCLIRequiresUnmanagedAdoption(t *testing.T) {
	cfg, input, path := setupFoundation(t)
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	record, err := s.Get(t.Context(), "example", "")
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.CreateReliable(t.Context(), record, execution.AdmissionOptions{Owner: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []execution.Operation{execution.StartStep, execution.StartAttempt} {
		original.Snapshot, err = s.ChangeReliable(t.Context(), execution.ReliableMutation{ID: original.Snapshot.ID, Owner: strings.Repeat("a", 32), Version: original.Snapshot.Version, Step: original.Snapshot.Steps[0].ID, Operation: op})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = executeWithRuntime(t.Context(), []string{"opsweave", "--config", cfg, "server"}, io.Discard, io.Discard, testCatalog(), nil); !errors.Is(err, execution.ErrUnmanaged) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	runCtx, stop := context.WithCancel(ctx)
	ready := &readyWriter{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- executeWithRuntime(runCtx, []string{"opsweave", "--config", cfg, "server", "--adopt-unmanaged"}, ready, io.Discard, testCatalog(), nil)
	}()
	select {
	case <-ready.ready:
	case err = <-done:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stop()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	snap, err := s.Execution(ctx, original.Snapshot.ID)
	if err != nil || snap.State != execution.Failed || !snap.UncertainEffects {
		t.Fatal(snap, err)
	}
	// An unfinished invocation is classified without replaying a handler.
}

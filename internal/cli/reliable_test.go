package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

func TestReliableCLIReplayAndIdempotency(t *testing.T) {
	cfg, input, path := setupFoundation(t)
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	fail := true
	calls := 0
	runtime, err := action.New([]action.Registration{{Descriptor: testCatalog()["test.echo"], Run: func(context.Context, map[string]any) (map[string]any, error) {
		calls++
		if fail {
			return nil, errors.New("private-action-error")
		}
		return map[string]any{}, nil
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (execution.Snapshot, error) {
		var out, diag bytes.Buffer
		err := executeWithRuntime(t.Context(), append([]string{"opsweave", "--config", cfg}, args...), &out, &diag, testCatalog(), runtime)
		var snap execution.Snapshot
		if out.Len() > 0 {
			if decodeErr := json.Unmarshal(out.Bytes(), &snap); decodeErr != nil {
				t.Fatal(decodeErr, out.String())
			}
		}
		return snap, err
	}
	original, err := invoke("run", "--json", "--idempotency-key", "initial", "example")
	if err != execution.ErrAction || original.State != execution.Failed {
		t.Fatal(original, err)
	}
	again, err := invoke("run", "--json", "--idempotency-key", "initial", "example")
	if err != execution.ErrAction || again.ID != original.ID || calls != 1 {
		t.Fatal(again, err, calls)
	}
	fail = false
	retried, err := invoke("execution", "retry", "--json", "--idempotency-key", "replay", original.ID)
	if err != nil || retried.Parent != original.ID || retried.ID == original.ID || retried.State != execution.Succeeded || calls != 2 {
		t.Fatal(retried, err, calls)
	}
	duplicate, err := invoke("execution", "retry", "--json", "--idempotency-key", "replay", original.ID)
	if err != nil || duplicate.ID != retried.ID || calls != 2 {
		t.Fatal(duplicate, err, calls)
	}
	if _, err := invoke("execution", "retry", "--json", retried.ID); err != execution.ErrInvalid {
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
	stored, err := s.Execution(t.Context(), retried.ID)
	if err != nil || stored.Format != 2 || len(stored.Steps[0].Attempts) != 1 {
		t.Fatal(stored, err)
	}
}

func TestReliableCLICancellationRequestAndOwnedShutdown(t *testing.T) {
	cfg, input, path := setupFoundation(t)
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	runtime, err := action.New([]action.Registration{{Descriptor: testCatalog()["test.echo"], Run: func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	result := make(chan error, 1)
	var out bytes.Buffer
	wg.Go(func() {
		result <- executeWithRuntime(ctx, []string{"opsweave", "--config", cfg, "run", "--json", "example"}, &out, io.Discard, testCatalog(), runtime)
	})
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("action did not start")
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
	list, err := s.Executions(t.Context())
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	var acknowledgement bytes.Buffer
	err = executeWithRuntime(t.Context(), []string{"opsweave", "--config", cfg, "execution", "cancel", "--json", "--step", "first", list[0].ID}, &acknowledgement, io.Discard, testCatalog(), runtime)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Status string          `json:"status"`
		State  execution.State `json:"state"`
	}
	if err := json.Unmarshal(acknowledgement.Bytes(), &receipt); err != nil || receipt.Status != "cancellation_requested" || receipt.State != execution.Running {
		t.Fatal(acknowledgement.String(), err)
	}
	select {
	case err := <-result:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("run did not join cancellation")
	}
	var snap execution.Snapshot
	if err := json.Unmarshal(out.Bytes(), &snap); err != nil || snap.State != execution.Cancelled {
		t.Fatal(out.String(), err)
	}
	for _, args := range [][]string{{"run", "--idempotency-key", "", "example"}, {"execution", "cancel", "--step", "", snap.ID}, {"execution", "retry", "--idempotency-key", "", snap.ID}, {"execution", "cancel", "--step", "unknown", snap.ID}} {
		if err := executeWithRuntime(t.Context(), append([]string{"opsweave", "--config", cfg}, args...), io.Discard, io.Discard, testCatalog(), runtime); err == nil {
			t.Fatal(args)
		}
	}
	for _, jsonOutput := range []bool{false, true} {
		if err := writeCancellation(executionBrokenWriter{}, jsonOutput, "cancellation_requested", snap, ""); err == nil {
			t.Fatal("acknowledgement output failure hidden")
		}
	}
}

func TestCancellationReceiptIdentifiesTerminalStep(t *testing.T) {
	snap := execution.Snapshot{ID: "0123456789abcdef0123456789abcdef", State: execution.Running, Steps: []execution.Step{{ID: "first", State: execution.Succeeded}}}
	var out bytes.Buffer
	if err := writeCancellation(&out, true, "cancellation_requested", snap, "first"); err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err := json.Unmarshal(out.Bytes(), &value); err != nil || value["status"] != "succeeded" || value["state"] != "running" || value["stepState"] != "succeeded" || value["step"] != "first" {
		t.Fatal(out.String(), err)
	}
}

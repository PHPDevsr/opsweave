package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
)

func TestExecutionCLIHistoryAndProductionBoundary(t *testing.T) {
	cfg, input, path := setupFoundation(t)
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run(t.Context(), []string{"opsweave", "--config", cfg, "run", "example"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "action version unavailable") {
		t.Fatal(code, &stdout, &stderr)
	}
	descriptor := testCatalog()["test.echo"]
	runtime, err := action.New([]action.Registration{{Descriptor: descriptor, Run: func(context.Context, map[string]any) (map[string]any, error) { return map[string]any{}, nil }}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (string, error) {
		stdout.Reset()
		stderr.Reset()
		err := executeWithRuntime(t.Context(), append([]string{"opsweave", "--config", cfg}, args...), &stdout, &stderr, testCatalog(), runtime)
		return stdout.String(), err
	}
	out, err := invoke("run", "--json", "example")
	if err != nil {
		t.Fatal(err)
	}
	var snap execution.Snapshot
	if err := json.Unmarshal([]byte(out), &snap); err != nil || snap.State != execution.Succeeded || !execution.ValidID(snap.ID) {
		t.Fatal(out, err)
	}
	if strings.Contains(out, "definition") || strings.Contains(out, "steps") {
		t.Fatal("run should return identifying metadata", out)
	}
	for _, args := range [][]string{{"execution", "get", snap.ID}, {"execution", "get", "--json", snap.ID}, {"execution", "list"}, {"execution", "list", "--json"}, {"execution", "logs", snap.ID}, {"execution", "logs", "--json", snap.ID}} {
		out, err := invoke(args...)
		if err != nil || !strings.Contains(out, snap.ID) {
			t.Fatal(args, out, err)
		}
	}
	for _, args := range [][]string{{"run"}, {"run", "--unknown", "example"}, {"execution", "get", "bad"}, {"execution", "logs", "bad"}, {"execution", "list", "extra"}, {"execution", "get", strings.Repeat("0", 32)}} {
		if _, err := invoke(args...); err == nil {
			t.Fatal(args)
		}
	}
	if err := executeWithRuntime(t.Context(), []string{"opsweave", "--config", cfg, "run", "example"}, io.Discard, io.Discard, testCatalog(), runtime); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	rows, err := store.Executions(t.Context())
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
}

type executionBrokenWriter struct{}

func (executionBrokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestExecutionCLIFailureAndOutputAfterCommit(t *testing.T) {
	cfg, input, path := setupFoundation(t)
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	for _, fails := range []bool{true, false} {
		r, err := action.New([]action.Registration{{Descriptor: testCatalog()["test.echo"], Run: func(context.Context, map[string]any) (map[string]any, error) {
			if fails {
				return nil, errors.New("private-action-secret")
			}
			return map[string]any{}, nil
		}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		var writer io.Writer = &out
		if !fails {
			writer = executionBrokenWriter{}
		}
		err = executeWithRuntime(t.Context(), []string{"opsweave", "--config", cfg, "run", "--json", "example"}, writer, io.Discard, testCatalog(), r)
		if err == nil || strings.Contains(err.Error(), "private-action-secret") {
			t.Fatal(err)
		}
		if fails && (!errors.Is(err, execution.ErrAction) || !strings.Contains(out.String(), `"state":"failed"`)) {
			t.Fatal(out.String(), err)
		}
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
	rows, err := s.Executions(t.Context())
	if err != nil || len(rows) != 2 || rows[0].State != execution.Succeeded {
		t.Fatal(rows, err)
	}
}

func TestExecutionPresentationAndDiagnostics(t *testing.T) {
	for _, v := range []any{execution.Snapshot{ID: "id", Definition: json.RawMessage(`{}`), Steps: []execution.Step{{ID: "one"}}}, []execution.Snapshot{{ID: "id"}}, []execution.Event{{Execution: "id"}}} {
		if err := writeExecution(executionBrokenWriter{}, false, v); err == nil {
			t.Fatal(v)
		}
		if err := writeExecution(executionBrokenWriter{}, true, v); err == nil {
			t.Fatal(v)
		}
	}
	err := executionDiagnostic(&execution.Failure{Cause: errors.New("private")})
	if strings.Contains(err.Error(), "private") {
		t.Fatal(err)
	}
	if executionDiagnostic(execution.ErrPermission) != execution.ErrPermission {
		t.Fatal("category lost")
	}
}

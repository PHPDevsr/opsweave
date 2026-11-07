package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "root", want: "USAGE:"},
		{name: "help flag", args: []string{"--help"}, want: "version"},
		{name: "short help", args: []string{"-h"}, want: "completion"},
		{name: "help command", args: []string{"help"}, want: "USAGE:"},
		{name: "version help", args: []string{"version", "--help"}, want: "--json"},
		{name: "help version", args: []string{"help", "version"}, want: "--json"},
		{name: "version", args: []string{"version"}, want: "opsweave version dev\n"},
		{name: "JSON version", args: []string{"version", "--json"}, want: "{\"version\":\"dev\"}\n"},
		{name: "JSON disabled", args: []string{"version", "--json=false"}, want: "opsweave version dev\n"},
		{name: "completion help", args: []string{"completion"}, want: "bash"},
		{name: "bash completion", args: []string{"completion", "bash"}, want: "opsweave"},
		{name: "zsh completion", args: []string{"completion", "zsh"}, want: "opsweave"},
		{name: "fish completion", args: []string{"completion", "fish"}, want: "opsweave"},
		{name: "PowerShell completion", args: []string{"completion", "pwsh"}, want: "Register-ArgumentCompleter"},
		{name: "completion request", args: []string{"version", "--j", "--generate-shell-completion"}, want: "--json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			args := append([]string{"opsweave"}, tt.args...)
			if code := Run(t.Context(), args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code = %d; stderr = %q", code, &stderr)
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Errorf("stdout = %q, want %q", &stdout, tt.want)
			}
			if stderr.Len() != 0 {
				t.Errorf("unexpected stderr: %q", &stderr)
			}
		})
	}
}

func TestRunInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"unknown"},
		{"--unknown"},
		{"version", "extra"},
		{"version", "--", "extra"},
		{"version", "--unknown"},
		{"version", "--json=invalid"},
		{"--json"},
		{"help", "unknown"},
		{"completion", "unknown"},
		{"completion", "--unknown"},
		{"completion", "bash", "--unknown"},
		{"completion", "bash", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := Run(t.Context(), append([]string{"opsweave"}, args...), &stdout, &stderr); code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			if stdout.Len() != 0 {
				t.Errorf("unexpected stdout: %q", &stdout)
			}
			if !strings.HasPrefix(stderr.String(), "Error: ") || strings.Count(stderr.String(), "\n") != 1 {
				t.Errorf("expected one diagnostic, got %q", &stderr)
			}
		})
	}
}

func TestRunIndependentInvocations(t *testing.T) {
	for _, args := range [][]string{
		{"version", "--json"},
		{"--generate-shell-completion"},
		{"version", "--unknown"},
	} {
		Run(t.Context(), append([]string{"opsweave"}, args...), io.Discard, io.Discard)
		var stdout, stderr bytes.Buffer
		if code := Run(t.Context(), []string{"opsweave", "version"}, &stdout, &stderr); code != 0 {
			t.Fatalf("exit code = %d; stderr = %q", code, &stderr)
		}
		if stdout.String() != "opsweave version dev\n" || stderr.Len() != 0 {
			t.Fatalf("state leaked: stdout = %q, stderr = %q", &stdout, &stderr)
		}
	}
}

func TestVersionJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run(t.Context(), []string{"opsweave", "version", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stderr = %q", code, &stderr)
	}
	var result map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result["version"] != "dev" {
		t.Fatalf("unexpected version response: %v", result)
	}
}

func TestOutputFailures(t *testing.T) {
	writeErr := errors.New("output unavailable")
	for _, args := range [][]string{
		{}, {"--help"}, {"help", "version"}, {"version"},
		{"version", "--json"}, {"completion", "bash"}, {"version", "--j", "--generate-shell-completion"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			args := append([]string{"opsweave"}, args...)
			if err := execute(t.Context(), args, failingWriter{err: writeErr}, io.Discard); !errors.Is(err, writeErr) {
				t.Errorf("error = %v, want %v", err, writeErr)
			}
			var stderr bytes.Buffer
			if code := Run(t.Context(), args, failingWriter{err: writeErr}, &stderr); code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			if strings.Count(stderr.String(), "\n") != 1 {
				t.Errorf("expected one diagnostic, got %q", &stderr)
			}
		})
	}
}

func TestShortOutput(t *testing.T) {
	err := execute(t.Context(), []string{"opsweave", "version"}, failingWriter{}, io.Discard)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("error = %v, want %v", err, io.ErrShortWrite)
	}
}

func TestDiagnosticFailure(t *testing.T) {
	if code := Run(t.Context(), []string{"opsweave", "unknown"}, io.Discard, failingWriter{err: io.ErrClosedPipe}); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdout, stderr bytes.Buffer
	if err := execute(ctx, []string{"opsweave", "version"}, &stdout, &stderr); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if code := Run(ctx, []string{"opsweave", "version"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 {
		t.Errorf("exit code = %d, stdout = %q", code, &stdout)
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

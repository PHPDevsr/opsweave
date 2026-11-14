package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecutionConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opsweave.yaml")
	if err := os.WriteFile(path, []byte("execution: {workers: 2, active_steps: 4, running_executions: 3, queued_executions: 8, queue_timeout: 2h, shutdown_timeout: 15s}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(t.Context(), path, dir, []string{"OPSWEAVE_EXECUTION_WORKERS=3", "OPSWEAVE_EXECUTION_QUEUE_TIMEOUT=10m"})
	if err != nil || c.Execution.Workers != 3 || c.Execution.ActiveSteps != 4 || c.Execution.QueueTimeout != 10*time.Minute || c.Execution.ShutdownTimeout != 15*time.Second {
		t.Fatal(c, err)
	}
	for _, input := range []string{"{workers: 0}", "{workers: 65}", "{active_steps: 257}", "{running_executions: 0}", "{queued_executions: 10001}", "{queue_timeout: 0s}", "{shutdown_timeout: 6m}", "{workers: true}", "{workers: '2'}", "{queue_timeout: 1}", "{unknown: private-marker}", "{workers: 2, workers: 3}"} {
		if err := os.WriteFile(path, []byte("execution: "+input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(t.Context(), path, dir, nil); err == nil || strings.Contains(err.Error(), "private-marker") {
			t.Fatal(input, err)
		}
	}
	for _, env := range []string{"OPSWEAVE_EXECUTION_WORKERS=private-marker", "OPSWEAVE_EXECUTION_WORKERS=0", "OPSWEAVE_EXECUTION_ACTIVE_STEPS=0", "OPSWEAVE_EXECUTION_RUNNING_EXECUTIONS=0", "OPSWEAVE_EXECUTION_QUEUED_EXECUTIONS=0", "OPSWEAVE_EXECUTION_SHUTDOWN_TIMEOUT=private-marker", "OPSWEAVE_EXECUTION_QUEUE_TIMEOUT=25h", "OPSWEAVE_EXECUTION_OTHER=private-marker"} {
		if _, err := Load(t.Context(), "", t.TempDir(), []string{env}); err == nil || strings.Contains(err.Error(), "private-marker") {
			t.Fatal(env, err)
		}
	}
}

func TestRetentionAgeConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opsweave.yaml")
	if c, err := Load(t.Context(), "", dir, nil); err != nil || c.RetentionAge != 0 {
		t.Fatal(c, err)
	}
	if err := os.WriteFile(path, []byte("execution: {retention_age: 720h}"), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(t.Context(), path, dir, nil); err != nil || c.RetentionAge != 720*time.Hour {
		t.Fatal(c, err)
	}
	if c, err := Load(t.Context(), path, dir, []string{"OPSWEAVE_EXECUTION_RETENTION_AGE=24h"}); err != nil || c.RetentionAge != 24*time.Hour {
		t.Fatal(c, err)
	}
	for _, input := range []string{"0s", "-1h", "24", "null", "private-marker"} {
		if err := os.WriteFile(path, []byte("execution: {retention_age: "+input+"}"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(t.Context(), path, dir, nil); err == nil || strings.Contains(err.Error(), "private-marker") {
			t.Fatal(input, err)
		}
	}
}

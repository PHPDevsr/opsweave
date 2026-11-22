package fileaction

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsCleanupFailureDoesNotReportCopySuccess(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "input"), []byte("published"))
	source, err := os.OpenRoot(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	}()
	destination, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := destination.Close(); err != nil {
			t.Error(err)
		}
	}()
	// The operator-controlled tree normally has no concurrent attribute writer.
	// Inject read-only state only in this private fixture to exercise post-link
	// cleanup failure without weakening native deletion's permission checks.
	transfer := func(ctx context.Context, w io.Writer, r io.Reader, limit int64) (int64, error) {
		n, err := copyBounded(ctx, w, r, limit)
		if err != nil {
			return n, err
		}
		return n, w.(*os.File).Chmod(0400)
	}
	_, err = copyFileWith(t.Context(), source, "input", destination, "output", 1<<20, transfer)
	if err == nil {
		t.Fatal("temporary cleanup failure reported success")
	}
	entries, readErr := os.ReadDir(dst)
	if readErr != nil {
		t.Fatal(readErr)
	}
	reserved := 0
	for _, entry := range entries {
		name := filepath.Join(dst, entry.Name())
		if strings.HasPrefix(entry.Name(), ".opsweave-") {
			reserved++
		}
		if err := os.Chmod(name, 0600); err != nil {
			t.Error(err)
		}
		if strings.HasPrefix(entry.Name(), ".opsweave-") {
			if err := os.Remove(name); err != nil {
				t.Error(err)
			}
		}
	}
	if reserved != 1 {
		t.Fatal("cleanup failure was not exercised", reserved, err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "output")); err != nil || string(data) != "published" {
		t.Fatal("published target was rolled back", data, err)
	}
	if _, err := os.Stat(filepath.Join(src, "input")); errors.Is(err, os.ErrNotExist) {
		t.Fatal("source removed")
	}
}

func TestWindowsNativeOpenPreservesRootCaseSemantics(t *testing.T) {
	r, src, dst := fixture(t)
	write(t, filepath.Join(src, "input"), []byte("case"))
	if out := result(t, r, "file.copy", copyInput("INPUT", "OUTPUT")); out["bytes"] != float64(4) {
		t.Fatal(out)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "output")); err != nil || string(data) != "case" {
		t.Fatal(data, err)
	}
	if out := result(t, r, "file.cleanup", map[string]any{"root": "destination", "path": "output"}); out["removed"] != true {
		t.Fatal(out)
	}
}

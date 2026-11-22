package fileaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

func fixture(t testing.TB) (*action.Runtime, string, string) {
	t.Helper()
	src, dst := t.TempDir(), t.TempDir()
	r := runtimeFor(t, Config{Enabled: true, Roots: []Root{{Name: "source", Path: src, AllowedActions: []string{"file.exists", "file.copy", "file.cleanup"}}, {Name: "destination", Path: dst, AllowedActions: []string{"file.exists", "file.copy", "file.cleanup"}}}})
	return r, src, dst
}

func runtimeFor(t testing.TB, c Config) *action.Runtime {
	t.Helper()
	entries, err := Registrations(c)
	if err != nil {
		t.Fatal(err)
	}
	var grants []string
	for _, entry := range entries {
		grants = append(grants, entry.Descriptor.ID)
	}
	r, err := action.New(entries, grants)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func write(t testing.TB, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func copyInput(source, dest string) map[string]any {
	return map[string]any{"source_root": "source", "source": source, "destination_root": "destination", "destination": dest}
}

func result(t *testing.T, r *action.Runtime, id string, in map[string]any) map[string]any {
	t.Helper()
	out, err := r.Invoke(t.Context(), id, "1", in)
	if err != nil {
		t.Fatal(id, err)
	}
	var value map[string]any
	if err := json.Unmarshal(out, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func noTemps(t testing.TB, dir string) {
	t.Helper()
	if err := filepath.WalkDir(dir, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".opsweave-") {
			t.Error("reserved temporary entry remains", name)
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
}

func link(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) && os.Getenv("CI") == "" && os.Getenv("GITHUB_ACTIONS") == "" {
			t.Skip("local Windows account lacks symlink privilege; native CI must execute this security test")
		}
		t.Fatal(err)
	}
}

func TestFilesystemPolicyAndPortablePaths(t *testing.T) {
	dir := t.TempDir()
	valid := Config{Enabled: true, Roots: []Root{{Name: "root", Path: dir, AllowedActions: []string{"file.exists"}}}}
	if valid.Validate() != nil {
		t.Fatal("valid config rejected")
	}
	if entries, err := Registrations(Config{}); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	for _, c := range []Config{
		{Enabled: true}, {MaxCopyBytes: -1}, {MaxCopyBytes: 1}, {MaxCopyBytes: 65 << 20}, {Roots: make([]Root, 129)},
		{Roots: []Root{{Name: "bad/name", Path: dir, AllowedActions: []string{"file.exists"}}}},
		{Roots: []Root{{Name: "root", Path: "relative", AllowedActions: []string{"file.exists"}}}},
		{Roots: []Root{{Name: "root", Path: dir, AllowedActions: []string{"file.exec"}}}},
		{Roots: []Root{{Name: "root", Path: dir}}},
		{Roots: []Root{{Name: "root", Path: dir, AllowedActions: []string{"file.exists", "file.exists"}}}},
		{Roots: []Root{valid.Roots[0], valid.Roots[0]}},
	} {
		if c.Validate() == nil {
			t.Fatal("bad configuration accepted", c)
		}
	}
	for _, name := range []string{"a", "a/b", "unicode-資料/file.txt", "COM0", "ordinary..name"} {
		if !validPath(name) {
			t.Fatal("safe path rejected", name)
		}
	}
	for _, name := range []string{"", ".", "..", "/absolute", "a/../b", "a//b", "a/./b", "a/", "C:/path", `a\b`, `\\host\share`, "a:b", "a\x00b", "a\nb", "a.", "a ", "a/*", "a?b", "a|b", "a<b", "a>b", `a"b`, "CON", "con.txt", "NUL", "AUX", "PRN", "COM1.txt", "com1 .txt", "LPT9", "COM¹", "CONIN$", ".opsweave-test", "a/.OPSWEAVE-test/b", strings.Repeat("a", 1025), strings.Repeat("a/", 32) + "a", "\xff"} {
		if validPath(name) {
			t.Fatal("unsafe path accepted", name)
		}
	}
}

func TestFilesystemExistsCopyCleanup(t *testing.T) {
	r, src, dst := fixture(t)
	data := []byte{0, 1, 2, 255, 128, 10}
	write(t, filepath.Join(src, "input.bin"), data)
	if err := os.Mkdir(filepath.Join(dst, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		root, path, kind string
		exists           bool
	}{{"source", "input.bin", "file", true}, {"destination", "sub", "directory", true}, {"destination", "missing", "missing", false}} {
		out := result(t, r, "file.exists", map[string]any{"root": scenario.root, "path": scenario.path})
		if len(out) != 2 || out["exists"] != scenario.exists || out["kind"] != scenario.kind {
			t.Fatal(out)
		}
	}
	out := result(t, r, "file.copy", copyInput("input.bin", "sub/output.bin"))
	if len(out) != 1 || out["bytes"] != float64(len(data)) {
		t.Fatal(out)
	}
	copied, err := os.ReadFile(filepath.Join(dst, "sub/output.bin"))
	if err != nil || !bytes.Equal(copied, data) {
		t.Fatal(copied, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dst, "sub/output.bin"))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal(info, err)
		}
	}
	for _, expected := range []bool{true, false} {
		out := result(t, r, "file.cleanup", map[string]any{"root": "destination", "path": "sub/output.bin"})
		if len(out) != 1 || out["removed"] != expected {
			t.Fatal(out)
		}
	}
	if out, err := r.Invoke(t.Context(), "file.cleanup", "1", map[string]any{"root": "destination", "path": "sub"}); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
		t.Fatal(out, err)
	}
	if info, err := os.Stat(filepath.Join(dst, "sub")); err != nil || !info.IsDir() {
		t.Fatal("cleanup removed directory", err)
	}
	noTemps(t, dst)
}

func TestFilesystemGrantsSnapshotAndRevocation(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "file"), []byte("secret-marker"))
	c := Config{Enabled: true, Roots: []Root{{Name: "root", Path: dir, AllowedActions: []string{"file.exists"}}}}
	entries, err := Registrations(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Roots[0].Name = "changed"
	c.Roots[0].Path = t.TempDir()
	c.Roots[0].AllowedActions[0] = "file.cleanup"
	r, err := action.New(entries, []string{"file.exists"})
	if err != nil {
		t.Fatal(err)
	}
	if out := result(t, r, "file.exists", map[string]any{"root": "root", "path": "file"}); out["exists"] != true {
		t.Fatal(out)
	}
	for _, input := range []map[string]any{{"root": "changed", "path": "file"}, {"root": "root", "path": "../file"}, {"root": "root", "path": "file", "flags": "secret-marker"}, {"root": 12, "path": "file"}} {
		if out, err := r.Invoke(t.Context(), "file.exists", "1", input); err == nil || len(out) != 0 || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal(out, err)
		}
	}
	if err := r.Check("file.cleanup", "1"); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := r.Check("file.exists", "2"); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatal(err)
	}
	denied, err := action.New(entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := denied.Invoke(t.Context(), "file.exists", "1", map[string]any{"root": "root", "path": "file"}); !errors.Is(err, execution.ErrPermission) {
		t.Fatal(err)
	}
	copyRuntime := runtimeFor(t, Config{Enabled: true, Roots: []Root{{Name: "source", Path: dir, AllowedActions: []string{"file.copy"}}, {Name: "destination", Path: t.TempDir(), AllowedActions: []string{"file.exists"}}}})
	if out, err := copyRuntime.Invoke(t.Context(), "file.copy", "1", copyInput("file", "copy")); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
		t.Fatal("copy needs both root grants", out, err)
	}
}

func TestFilesystemCopyBoundsAndExistingTargets(t *testing.T) {
	r, src, dst := fixture(t)
	write(t, filepath.Join(src, "input"), []byte("input"))
	write(t, filepath.Join(src, "empty"), nil)
	if err := os.Mkdir(filepath.Join(src, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dst, "existing"), []byte("preserve"))
	if err := os.Mkdir(filepath.Join(dst, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"existing", "directory", "missing/child"} {
		if out, err := r.Invoke(t.Context(), "file.copy", "1", copyInput("input", target)); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
			t.Fatal(target, out, err)
		}
	}
	for _, source := range []string{"missing", "directory"} {
		if out, err := r.Invoke(t.Context(), "file.copy", "1", copyInput(source, "blocked-source")); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
			t.Fatal(source, out, err)
		}
	}
	if out := result(t, r, "file.copy", copyInput("empty", "empty")); out["bytes"] != float64(0) {
		t.Fatal(out)
	}
	if info, err := os.Stat(filepath.Join(dst, "empty")); err != nil || info.Size() != 0 {
		t.Fatal("empty source was not copied", info, err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "existing")); err != nil || string(data) != "preserve" {
		t.Fatal(data, err)
	}
	r = runtimeFor(t, Config{Enabled: true, MaxCopyBytes: 1 << 20, Roots: []Root{{Name: "source", Path: src, AllowedActions: []string{"file.copy"}}, {Name: "destination", Path: dst, AllowedActions: []string{"file.copy"}}}})
	write(t, filepath.Join(src, "large"), bytes.Repeat([]byte("x"), (1<<20)+1))
	if out, err := r.Invoke(t.Context(), "file.copy", "1", copyInput("large", "oversize")); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
		t.Fatal(out, err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "oversize")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("oversize target published", err)
	}
	write(t, filepath.Join(src, "boundary"), bytes.Repeat([]byte("x"), 1<<20))
	if out := result(t, r, "file.copy", copyInput("boundary", "boundary")); out["bytes"] != float64(1<<20) {
		t.Fatal(out)
	}
	noTemps(t, dst)
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(p []byte) (int, error) { return f(p) }

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) { return f(p) }

func TestBoundedCopyFailureGrowthAndCancellation(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		reader io.Reader
		writer io.Writer
		limit  int64
	}{
		{"growth", strings.NewReader("12345"), io.Discard, 4},
		{"read", readFunc(func([]byte) (int, error) { return 0, errors.New("read failed") }), io.Discard, 4},
		{"write", strings.NewReader("1234"), writeFunc(func([]byte) (int, error) { return 0, errors.New("write failed") }), 4},
		{"short", strings.NewReader("1234"), writeFunc(func([]byte) (int, error) { return 1, nil }), 4},
		{"no_progress", readFunc(func([]byte) (int, error) { return 0, nil }), io.Discard, 4},
		{"bad_reader", readFunc(func([]byte) (int, error) { return -1, nil }), io.Discard, 4},
		{"bad_writer", strings.NewReader("1234"), writeFunc(func([]byte) (int, error) { return -1, nil }), 4},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := copyBounded(t.Context(), scenario.writer, scenario.reader, scenario.limit); err == nil {
				t.Fatal("copy failure hidden")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var destination bytes.Buffer
	source := readFunc(func(p []byte) (int, error) { cancel(); return copy(p, "data"), nil })
	if _, err := copyBounded(ctx, &destination, source, 4); !errors.Is(err, context.Canceled) || destination.Len() != 0 {
		t.Fatal("write after cancellation", destination.String(), err)
	}
	var exact bytes.Buffer
	if count, err := copyBounded(t.Context(), &exact, strings.NewReader("data"), 4); err != nil || count != 4 || exact.String() != "data" {
		t.Fatal(count, err)
	}
}

func TestFilesystemCanceledActionsDoNotMutate(t *testing.T) {
	r, src, dst := fixture(t)
	write(t, filepath.Join(src, "input"), []byte("unchanged"))
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		} else {
			cancel()
		}
		defer cancel()
		for _, call := range []struct {
			id string
			in map[string]any
		}{{"file.copy", copyInput("input", "output")}, {"file.cleanup", map[string]any{"root": "source", "path": "input"}}} {
			if out, err := r.Invoke(ctx, call.id, "1", call.in); err == nil || len(out) != 0 {
				t.Fatal(out, err)
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(src, "input")); err != nil || string(data) != "unchanged" {
		t.Fatal(data, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "output")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	noTemps(t, dst)
}

func TestFilesystemExclusiveConcurrentPublication(t *testing.T) {
	r, src, dst := fixture(t)
	write(t, filepath.Join(src, "input"), []byte("same contents"))
	var owners sync.WaitGroup
	results := make(chan error, 16)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for range 16 {
		owners.Go(func() { _, err := r.Invoke(ctx, "file.copy", "1", copyInput("input", "winner")); results <- err })
	}
	owners.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, execution.ErrAction) {
			t.Error(err)
		}
	}
	if success != 1 {
		t.Fatal("publication was not exclusive", success)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "winner")); err != nil || string(data) != "same contents" {
		t.Fatal(data, err)
	}
	noTemps(t, dst)
}

func TestFilesystemSymlinkBoundaries(t *testing.T) {
	r, src, dst := fixture(t)
	outside := t.TempDir()
	write(t, filepath.Join(src, "input"), []byte("inside"))
	write(t, filepath.Join(outside, "sentinel"), []byte("outside-secret-marker"))
	if err := os.Mkdir(filepath.Join(src, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(src, "sub", "input"), []byte("safe"))
	link(t, "sub", filepath.Join(src, "inside"))
	link(t, outside, filepath.Join(src, "escape"))
	link(t, filepath.Join(outside, "sentinel"), filepath.Join(src, "final"))
	if out := result(t, r, "file.exists", map[string]any{"root": "source", "path": "inside/input"}); out["exists"] != true {
		t.Fatal(out)
	}
	for _, name := range []string{"escape/sentinel", "final"} {
		for _, id := range []string{"file.exists", "file.cleanup", "file.copy"} {
			input := map[string]any{"root": "source", "path": name}
			if id == "file.copy" {
				input = copyInput(name, "blocked")
			}
			if out, err := r.Invoke(t.Context(), id, "1", input); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
				t.Fatal(name, id, out, err)
			}
		}
	}
	link(t, filepath.Join(outside, "sentinel"), filepath.Join(dst, "existing-link"))
	if out, err := r.Invoke(t.Context(), "file.copy", "1", copyInput("input", "existing-link")); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
		t.Fatal(out, err)
	}
	if data, err := os.ReadFile(filepath.Join(outside, "sentinel")); err != nil || string(data) != "outside-secret-marker" {
		t.Fatal("outside target changed", data, err)
	}
	noTemps(t, dst)
}

func FuzzFilesystemPath(f *testing.F) {
	for _, name := range []string{"safe/file", "../outside", "NUL", ".opsweave-temp", "unicode-資料"} {
		f.Add(name)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 65536 {
			return
		}
		if validPath(name) {
			if filepath.IsAbs(name) || strings.Contains(name, "\\") || strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
				t.Fatal("unsafe native path accepted")
			}
		}
	})
}

func BenchmarkFilesystemCopy(b *testing.B) {
	r, src, dst := fixture(b)
	write(b, filepath.Join(src, "input"), bytes.Repeat([]byte("x"), 4096))
	b.ReportAllocs()
	b.SetBytes(4096)
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.Invoke(b.Context(), "file.copy", "1", copyInput("input", "copy")); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		if err := os.Remove(filepath.Join(dst, "copy")); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

func TestFilesystemRootChangedAfterRegistration(t *testing.T) {
	r, src, _ := fixture(t)
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	write(t, src, []byte("not a directory"))
	if out, err := r.Invoke(t.Context(), "file.exists", "1", map[string]any{"root": "source", "path": "file"}); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
		t.Fatal(out, err)
	}
}

func TestFilesystemTransferFailureCleansTemporaryFiles(t *testing.T) {
	for _, scenario := range []string{"transfer", "flush", "cancel", "growth", "publish_conflict"} {
		t.Run(scenario, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			write(t, filepath.Join(src, "input"), []byte("data"))
			source, err := os.OpenRoot(src)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := source.Close(); err != nil {
					t.Error(err)
				}
			}()
			dest, err := os.OpenRoot(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := dest.Close(); err != nil {
					t.Error(err)
				}
			}()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			transfer := func(ctx context.Context, w io.Writer, r io.Reader, limit int64) (int64, error) {
				n, err := copyBounded(ctx, w, r, limit)
				if err != nil {
					return n, err
				}
				switch scenario {
				case "transfer":
					return n, errors.New("injected I/O failure")
				case "flush":
					if err := w.(*os.File).Close(); err != nil {
						t.Error(err)
					}
				case "cancel":
					cancel()
				case "growth":
					f, err := os.OpenFile(filepath.Join(src, "input"), os.O_APPEND|os.O_WRONLY, 0)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := f.WriteString("grew"); err != nil {
						t.Error(err)
					}
					if err := f.Close(); err != nil {
						t.Error(err)
					}
				case "publish_conflict":
					write(t, filepath.Join(dst, "output"), []byte("preserve"))
				}
				return n, nil
			}
			if _, err := copyFileWith(ctx, source, "input", dest, "output", 1<<20, transfer); err == nil {
				t.Fatal("failed copy reported success")
			}
			if scenario == "publish_conflict" {
				if data, err := os.ReadFile(filepath.Join(dst, "output")); err != nil || string(data) != "preserve" {
					t.Fatal(data, err)
				}
			} else if _, err := os.Lstat(filepath.Join(dst, "output")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed copy published a target", err)
			}
			noTemps(t, dst)
		})
	}
}

func TestFilesystemSymlinkSwapCannotEscapeRoot(t *testing.T) {
	r, src, dst := fixture(t)
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "safe"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dst, "safe"), 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(src, "safe", "value"), []byte("inside"))
	write(t, filepath.Join(src, "safe", "delete"), []byte("inside"))
	write(t, filepath.Join(outside, "value"), []byte("outside-secret-marker"))
	write(t, filepath.Join(outside, "delete"), []byte("preserve"))
	pivot := filepath.Join(src, "pivot")
	link(t, "safe", pivot)
	destPivot := filepath.Join(dst, "pivot")
	link(t, "safe", destPivot)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			for _, target := range []string{outside, "safe"} {
				select {
				case <-stop:
					return
				default:
				}
				for _, current := range []string{pivot, destPivot} {
					if err := os.Remove(current); err != nil && !errors.Is(err, os.ErrNotExist) {
						t.Error(err)
						return
					}
					if err := os.Symlink(target, current); err != nil {
						t.Error(err)
						return
					}
				}
			}
		}
	}()
	var stopOnce sync.Once
	join := func() { stopOnce.Do(func() { close(stop) }); <-done }
	defer join()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for i := range 50 {
		name := fmt.Sprintf("copy-%d", i)
		out, err := r.Invoke(ctx, "file.copy", "1", copyInput("pivot/value", "pivot/"+name))
		if err == nil {
			data, err := os.ReadFile(filepath.Join(dst, "safe", name))
			if err != nil || string(data) != "inside" {
				t.Fatal("copied outside root", string(out), data, err)
			}
		} else if !errors.Is(err, execution.ErrAction) {
			t.Fatal(err)
		}
		if _, err := r.Invoke(ctx, "file.cleanup", "1", map[string]any{"root": "source", "path": "pivot/delete"}); err != nil && !errors.Is(err, execution.ErrAction) {
			t.Fatal(err)
		}
	}
	join()
	if data, err := os.ReadFile(filepath.Join(outside, "delete")); err != nil || string(data) != "preserve" {
		t.Fatal("cleanup escaped root", data, err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 2 {
		t.Fatal("publication escaped destination root", entries, err)
	}
	noTemps(t, dst)
}

func TestNativeUnlinkCannotRemoveDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	}()
	f, err := r.Open("child")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := unlinkRegular(r, "child", f); err == nil {
		t.Fatal("native unlink accepted a directory")
	}
	if info, err := os.Stat(filepath.Join(dir, "child")); err != nil || !info.IsDir() {
		t.Fatal("directory removed", err)
	}
}

func TestNativeOpenDoesNotFollowFinalSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	write(t, filepath.Join(outside, "sentinel"), []byte("outside"))
	link(t, filepath.Join(outside, "sentinel"), filepath.Join(dir, "link"))
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	}()
	if file, err := openNoFollow(r, "link"); err == nil {
		// Windows can return a metadata handle to the reparse point itself.
		// The caller's regular-file/identity guard must reject that handle;
		// it must never describe the external regular-file target.
		info, statErr := file.Stat()
		if closeErr := file.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		if statErr != nil || info.Mode().IsRegular() {
			t.Fatal("native open followed final symlink", info, statErr)
		}
	}
}

package shellaction

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

func fixture(t testing.TB) Profile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "./testdata/fixture")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, output)
	}
	return Profile{Name: "fixture", Executable: path, WorkingDirectory: t.TempDir()}
}

func executor(t testing.TB, p Profile) *action.Runtime {
	t.Helper()
	entries, err := Registrations(Config{Enabled: true, Commands: []Profile{p}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := action.New(entries, []string{"shell.exec"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func input(args ...string) map[string]any {
	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg
	}
	return map[string]any{"command": "fixture", "args": values}
}

func TestProfileValidationAndPermissions(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	good := Profile{Name: "safe", Executable: self, WorkingDirectory: t.TempDir()}
	if err := (Config{}).Validate(); err != nil {
		t.Fatal(err)
	}
	if entries, err := Registrations(Config{}); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	for _, edit := range []func(*Profile){
		func(p *Profile) { p.Name = "" }, func(p *Profile) { p.Name = "bad name" }, func(p *Profile) { p.Name = ".." },
		func(p *Profile) { p.Executable = "relative" }, func(p *Profile) { p.Executable = filepath.Join(p.WorkingDirectory, "secret-marker") },
		func(p *Profile) { p.Executable = p.WorkingDirectory }, func(p *Profile) { p.WorkingDirectory = self },
		func(p *Profile) { p.Args = make([]string, 65) }, func(p *Profile) { p.Args = []string{strings.Repeat("x", 8193)} },
		func(p *Profile) { p.Args = []string{"bad\x00arg"} }, func(p *Profile) { p.Args = []string{string([]byte{0xff})} },
		func(p *Profile) { p.WorkingDirectory = "relative" }, func(p *Profile) { p.Name = strings.Repeat("x", 65) },
	} {
		p := good
		edit(&p)
		if err := (Config{Commands: []Profile{p}}).Validate(); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal("invalid/unsafe profile error", err)
		}
	}
	for _, c := range []Config{{Enabled: true}, {Commands: make([]Profile, 129)}, {Commands: []Profile{good, good}}} {
		if c.Validate() == nil {
			t.Fatal("invalid config accepted")
		}
	}
	entries, err := Registrations(Config{Enabled: true, Commands: []Profile{good}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := action.New(entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Check("shell.exec", "1") != execution.ErrPermission || r.Check("shell.exec", "2") != execution.ErrUnavailable {
		t.Fatal("permission/version bypass")
	}
	for _, path := range []string{"relative", `\\server\share`, "/tmp/bad:stream", "/tmp/bad\x00path"} {
		if validPath(path) {
			t.Fatal("unsafe path accepted")
		}
	}
}

func TestArgumentPreservationAndEnvironment(t *testing.T) {
	t.Setenv("OPSWEAVE_WEBHOOK_SECRET", "secret-marker")
	t.Setenv("SHELL_PRIVATE_MARKER", "secret-marker")
	p := fixture(t)
	p.Args = []string{"report"}
	r := executor(t, p)
	args := []string{"a b", `a"b`, "; touch forbidden", `$(id)`, "& whoami", `trailing\`, "line\nbreak", "", "\u00e9\u6f22\U0001f680"}
	out, err := r.Invoke(t.Context(), "shell.exec", "1", input(args...))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Stdout string `json:"stdout"`
		Exit   int    `json:"exit_code"`
	}
	if err := json.Unmarshal(out, &result); err != nil || result.Exit != 0 {
		t.Fatal(string(out), err)
	}
	var report struct {
		Args []string
		Cwd  string
		Env  []string
	}
	if err := json.Unmarshal([]byte(result.Stdout), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Args) != len(args) {
		t.Fatal(report.Args)
	}
	for i := range args {
		if args[i] != report.Args[i] {
			t.Fatalf("argv[%d] changed", i)
		}
	}
	want, err := filepath.EvalSymlinks(p.WorkingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(report.Cwd)
	if err != nil || got != want {
		t.Fatal(report.Cwd, err)
	}
	for _, entry := range report.Env {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "LANG", "LC_ALL", "TMPDIR", "SYSTEMROOT", "WINDIR", "TEMP", "TMP":
		default:
			t.Fatalf("unexpected environment key %q", key)
		}
	}
	if strings.Contains(result.Stdout, "secret-marker") {
		t.Fatal("parent environment leaked")
	}
	vars := map[string]string{}
	for _, entry := range report.Env {
		key, value, _ := strings.Cut(entry, "=")
		vars[strings.ToUpper(key)] = value
	}
	if runtime.GOOS == "windows" {
		if len(report.Env) != 4 || len(vars) != 4 || !filepath.IsAbs(vars["SYSTEMROOT"]) || vars["WINDIR"] != vars["SYSTEMROOT"] || vars["TEMP"] != os.TempDir() || vars["TMP"] != vars["TEMP"] {
			t.Fatal("Windows environment contract changed")
		}
	} else if len(report.Env) != 3 || len(vars) != 3 || vars["LANG"] != "C" || vars["LC_ALL"] != "C" || vars["TMPDIR"] != os.TempDir() {
		t.Fatal("Unix environment contract changed")
	}
}

func TestSnapshotAndArgumentLimits(t *testing.T) {
	p := fixture(t)
	p.Args = []string{"echo", "prefix"}
	c := Config{Enabled: true, Commands: []Profile{p}}
	entries, err := Registrations(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Commands[0].Args[1] = "changed"
	r, err := action.New(entries, []string{"shell.exec"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Invoke(t.Context(), "shell.exec", "1", input("suffix"))
	if err != nil || !strings.Contains(string(out), "prefix|suffix") {
		t.Fatal(string(out), err)
	}
	p.Args = []string{"echo"}
	r = executor(t, p)
	for _, args := range [][]string{make([]string, 63), {strings.Repeat("x", 8188)}} {
		if _, err := r.Invoke(t.Context(), "shell.exec", "1", input(args...)); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{make([]string, 64), {strings.Repeat("x", 8189)}, {"bad\x00arg"}, {string([]byte{0xff})}} {
		if _, err := r.Invoke(t.Context(), "shell.exec", "1", input(args...)); err == nil {
			t.Fatal("argument bound bypass")
		}
	}
	for _, value := range []map[string]any{{"command": "missing"}, {"command": "fixture", "unknown": "secret-marker"}, {"command": "fixture", "args": "not-array"}} {
		if _, err := r.Invoke(t.Context(), "shell.exec", "1", value); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestCaptureFailureAndOutputLimits(t *testing.T) {
	p := fixture(t)
	r := executor(t, p)
	for _, tc := range []struct {
		args []string
		want error
	}{
		{[]string{"echo", "ok"}, nil}, {[]string{"output", "8192", "8192", "text"}, nil},
		{[]string{"output", "8193", "0", "text"}, execution.ErrAction}, {[]string{"output", "0", "8193", "text"}, execution.ErrAction},
		{[]string{"output", "8192", "8192", "null"}, execution.ErrOutput}, {[]string{"binary"}, execution.ErrAction}, {[]string{"failure"}, execution.ErrAction},
	} {
		t.Run(strings.Join(tc.args, "-"), func(t *testing.T) {
			out, err := r.Invoke(t.Context(), "shell.exec", "1", input(tc.args...))
			if !errors.Is(err, tc.want) || strings.Contains(string(out), "secret-marker") {
				t.Fatal(string(out), err)
			}
			if err != nil && out != nil {
				t.Fatal("failed output persisted")
			}
		})
	}
}

func TestCancellationDeadlineAndChildCleanup(t *testing.T) {
	p := fixture(t)
	r := executor(t, p)
	for _, mode := range []string{"hold", "tree"} {
		for _, deadline := range []bool{false, true} {
			t.Run(mode+map[bool]string{true: "-deadline", false: "-cancel"}[deadline], func(t *testing.T) {
				listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				if err := listener.SetDeadline(time.Now().Add(8 * time.Second)); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				if deadline {
					var stop context.CancelFunc
					ctx, stop = context.WithTimeout(ctx, 3*time.Second)
					defer stop()
				}
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
					_, err := r.Invoke(ctx, "shell.exec", "1", input(mode, listener.Addr().String()))
					done <- err
				}()
				conn, err := listener.Accept()
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				if err := conn.SetReadDeadline(time.Now().Add(8 * time.Second)); err != nil {
					t.Fatal(err)
				}
				if pid, err := bufio.NewReader(conn).ReadString('\n'); err != nil || strings.TrimSpace(pid) == "" {
					t.Fatal(pid, err)
				}
				if !deadline {
					cancel()
				}
				select {
				case err := <-done:
					want := context.Canceled
					if deadline {
						want = context.DeadlineExceeded
					}
					if !errors.Is(err, want) {
						t.Fatal(err)
					}
				case <-time.After(6 * time.Second):
					t.Fatal("process owner did not finish")
				}
				var data [1]byte
				_, err = conn.Read(data[:])
				if err == nil {
					t.Fatal("child connection remained open")
				}
				if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					t.Fatal("child survived cancellation")
				}
			})
		}
	}
}

func TestProcessStartFailureAndProfileRecheck(t *testing.T) {
	p := Profile{Name: "fixture", Executable: filepath.Join(t.TempDir(), "invalid.exe"), WorkingDirectory: t.TempDir()}
	if err := os.WriteFile(p.Executable, []byte("not an executable"), 0700); err != nil {
		t.Fatal(err)
	}
	r := executor(t, p)
	if _, err := r.Invoke(t.Context(), "shell.exec", "1", input()); err != execution.ErrAction {
		t.Fatal(err)
	}
	if err := os.Remove(p.Executable); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Invoke(t.Context(), "shell.exec", "1", input()); err != execution.ErrAction {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.Invoke(ctx, "shell.exec", "1", input()); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestExitedRootCannotLeaveCapturedChildRunning(t *testing.T) {
	p := fixture(t)
	r := executor(t, p)
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(8 * time.Second)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
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
		_, err := r.Invoke(ctx, "shell.exec", "1", input("orphan", listener.Addr().String()))
		done <- err
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
	select {
	case err := <-done:
		if err != execution.ErrAction {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("inherited output handles blocked forever")
	}
	if _, err := io.Copy(io.Discard, reader); err != nil && !(runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10054))) {
		t.Fatal("child survived root exit", err)
	}
}

func FuzzShellArguments(f *testing.F) {
	for _, s := range []string{"", "normal", "; touch file", "\x00", "a b", string([]byte{0xff})} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 16384 {
			return
		}
		if validArgs([]string{s}) && (!utf8.ValidString(s) || strings.ContainsRune(s, 0) || len(s) > 8192) {
			t.Fatal("invalid arguments accepted")
		}
		_ = validName(s)
	})
}

func BenchmarkShellExecution(b *testing.B) {
	p := fixture(b)
	p.Args = []string{"echo", "ok"}
	r := executor(b, p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := r.Invoke(b.Context(), "shell.exec", "1", input())
		if err != nil || !strings.Contains(string(out), `"stdout":"ok"`) {
			b.Fatal(string(out), err)
		}
	}
}

var _ io.Writer = (*capture)(nil)

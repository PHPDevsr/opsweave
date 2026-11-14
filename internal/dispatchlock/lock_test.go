package dispatchlock

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

func TestLockChild(t *testing.T) {
	path := os.Getenv("OPSWEAVE_TEST_LOCK_PATH")
	if path == "" {
		return
	}
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(os.Stdout, "held\n"); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if _, err = os.Stdin.Read(b[:]); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeLockProcessExitAndAlias(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLockChild$")
	command.Env = append(os.Environ(), "OPSWEAVE_TEST_LOCK_PATH="+path)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
		_ = stdin.Close()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "held\n" {
		t.Fatal(line, err)
	}
	for _, candidate := range []string{path, filepath.Join(filepath.Dir(path), ".", filepath.Base(path))} {
		if lock, err := Acquire(candidate); !errors.Is(err, execution.ErrBusy) {
			if lock != nil {
				_ = lock.Close()
			}
			t.Fatal("second process acquired lock", err)
		}
	}
	if err = command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = command.Wait(); err == nil {
		t.Fatal("killed process unexpectedly succeeded")
	}
	waited = true
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal("lock not released by process exit", err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(path), "alias.sqlite")
	if err = os.Link(path, alias); err != nil {
		t.Skipf("hard-link privilege unavailable: %v", err)
	}
	if lock, err = Acquire(alias); !errors.Is(err, execution.ErrInvalid) {
		if lock != nil {
			_ = lock.Close()
		}
		t.Fatal("hard link accepted", err)
	}
}

func TestLockRejectsUnsafePathAndSidecar(t *testing.T) {
	for _, path := range []string{"", ":memory:", "file:bad", "x?pragma=yes"} {
		if lock, err := Acquire(path); err == nil {
			_ = lock.Close()
			t.Fatal(path)
		}
	}
	path := filepath.Join(t.TempDir(), "db.sqlite")
	if err := os.Mkdir(path+".dispatcher.lock", 0700); err != nil {
		t.Fatal(err)
	}
	if lock, err := Acquire(path); err == nil {
		_ = lock.Close()
		t.Fatal("directory sidecar accepted")
	}
}

func TestDirectoryAliasSharesNativeOwnership(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlink privilege unavailable: %v", err)
	}
	lock, err := Acquire(filepath.Join(real, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	other, err := Acquire(filepath.Join(alias, "db.sqlite"))
	if !errors.Is(err, execution.ErrBusy) {
		if other != nil {
			_ = other.Close()
		}
		t.Fatal("directory alias created separate ownership", err)
	}
}

func TestHardLinkedSidecarIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	side := lock.Path() + ".dispatcher.lock"
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Link(side, path+".alias"); err != nil {
		t.Skipf("hard-link privilege unavailable: %v", err)
	}
	lock, err = Acquire(path)
	if !errors.Is(err, execution.ErrInvalid) {
		if lock != nil {
			_ = lock.Close()
		}
		t.Fatal("hard-linked sidecar accepted", err)
	}
}

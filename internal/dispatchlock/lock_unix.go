//go:build linux || darwin

package dispatchlock

import (
	"errors"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"syscall"
)

func canonicalFile(f *os.File, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	actual, err := f.Stat()
	if err != nil {
		return "", err
	}
	st, ok := actual.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !os.SameFile(info, actual) || !ok || st.Nlink != 1 {
		return "", execution.ErrInvalid
	}
	return filepath.EvalSymlinks(path)
}
func tryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return execution.ErrBusy
	}
	return err
}
func unlock(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }

func privateSidecar(f *os.File, _ string) error { return f.Chmod(0600) }

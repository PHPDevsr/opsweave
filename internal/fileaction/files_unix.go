//go:build linux || darwin

package fileaction

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func readFlags() int { return os.O_RDONLY | syscall.O_NONBLOCK | syscall.O_NOFOLLOW }

func openNoFollow(parent *os.Root, name string) (file *os.File, resultErr error) {
	dir, err := parent.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, dir.Close())
		if resultErr != nil && file != nil {
			resultErr = errors.Join(resultErr, file.Close())
			file = nil
		}
	}()
	fd, err := unix.Openat(int(dir.Fd()), name, readFlags()|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "filesystem-local"), nil
}

func unlinkRegular(parent *os.Root, name string, _ *os.File) (resultErr error) {
	dir, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, dir.Close()) }()
	// Unlike Root.Remove, unlinkat without AT_REMOVEDIR can never delete a
	// directory if the final entry changes after regular-file validation.
	return unix.Unlinkat(int(dir.Fd()), name, 0)
}

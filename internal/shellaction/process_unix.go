//go:build linux || darwin

package shellaction

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type processOwner struct {
	cmd    *exec.Cmd
	budget cleanupBudget
}

func prepare(cmd *exec.Cmd) (*processOwner, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	owner := &processOwner{cmd: cmd}
	cmd.Cancel = func() error { owner.budget.until(); return owner.kill() }
	return owner, nil
}

func (o *processOwner) bind(*exec.Cmd) error { return nil }
func (o *processOwner) kill() error {
	if o.cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-o.cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func (o *processOwner) close() error {
	if o.cmd.Process == nil {
		return nil
	}
	deadline := o.budget.until()
	if err := o.kill(); err != nil {
		return err
	}
	for {
		err := syscall.Kill(-o.cmd.Process.Pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return err
		}
		if !time.Now().Before(deadline) {
			return errCommand
		}
		time.Sleep(time.Millisecond)
	}
}
func executable(info os.FileInfo) bool { return info.Mode().Perm()&0111 != 0 }
func environment() ([]string, error) {
	return []string{"LANG=C", "LC_ALL=C", "TMPDIR=" + os.TempDir()}, nil
}

package shellaction

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsSuspendedNonBreakawayOwnership(t *testing.T) {
	p := fixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Executable, "echo", "ok")
	var err error
	cmd.Env, err = environment()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := prepare(cmd)
	if err != nil {
		t.Fatal(err)
	}
	cleanupWindowsTest(t, cmd, owner, cancel)
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED == 0 {
		t.Fatal("ownership starts too late")
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(owner.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		t.Fatal(err)
	}
	if limits.BasicLimitInformation.LimitFlags != windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE {
		t.Fatal("breakaway or missing kill-on-close")
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := owner.bind(cmd); err != nil {
		t.Error(err)
		if killErr := cmd.Process.Kill(); killErr != nil {
			t.Error(killErr)
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Error(err)
	}
}

func TestWindowsAssignmentFailureCannotRunExecutable(t *testing.T) {
	p := fixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Executable, "echo", "must-not-run")
	var err error
	cmd.Env, err = environment()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := prepare(cmd)
	if err != nil {
		t.Fatal(err)
	}
	cleanupWindowsTest(t, cmd, owner, cancel)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Simulate a native ownership failure before the suspended executable runs.
	if err := windows.CloseHandle(owner.job); err != nil {
		t.Fatal(err)
	}
	owner.job = 0
	if err := owner.bind(cmd); err == nil {
		t.Fatal("unowned process was resumed")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	var exit *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &exit) {
		t.Fatal("suspended executable completed normally", err)
	}
}

func cleanupWindowsTest(t *testing.T, cmd *exec.Cmd, owner *processOwner, cancel context.CancelFunc) {
	t.Helper()
	t.Cleanup(func() {
		cancel()
		if cmd.Process != nil && cmd.ProcessState == nil {
			if err := cmd.Wait(); err == nil {
				t.Error("expected cancelled native test process")
			}
		}
		if owner.job != 0 {
			if err := owner.close(); err != nil {
				t.Error(err)
			}
		}
	})
}

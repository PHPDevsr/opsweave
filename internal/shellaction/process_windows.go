package shellaction

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processOwner struct {
	job    windows.Handle
	budget cleanupBudget
}

func prepare(cmd *exec.Cmd) (*processOwner, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(job))
	}
	owner := &processOwner{job: job}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED, HideWindow: true}
	cmd.Cancel = func() error {
		owner.budget.until()
		jobErr := windows.TerminateJobObject(job, 1)
		// Covers cancellation racing with assignment while the root is suspended.
		rootErr := cmd.Process.Kill()
		if errors.Is(rootErr, os.ErrProcessDone) {
			rootErr = nil
		}
		return errors.Join(jobErr, rootErr)
	}
	return owner, nil
}

func (o *processOwner) bind(cmd *exec.Cmd) (resultErr error) {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(process)) }()
	if err := windows.AssignProcessToJobObject(o.job, process); err != nil {
		return err
	}
	// No executable instructions have run. Exactly one initial thread exists.
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(snapshot)) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(cmd.Process.Pid) {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME|windows.THREAD_QUERY_LIMITED_INFORMATION, false, entry.ThreadID)
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(thread)) }()
		module, err := windows.LoadLibraryEx("kernel32.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, windows.FreeLibrary(module)) }()
		getPID, err := windows.GetProcAddress(module, "GetProcessIdOfThread")
		if err != nil {
			return err
		}
		pid, _, callErr := syscall.SyscallN(getPID, uintptr(thread))
		if pid == 0 {
			if callErr == 0 {
				return errCommand
			}
			return callErr
		}
		if uint32(pid) != uint32(cmd.Process.Pid) {
			return errCommand
		}
		count, err := windows.ResumeThread(thread)
		if err != nil {
			return err
		}
		if count != 1 {
			return errCommand
		}
		return nil
	}
	return errCommand
}

func (o *processOwner) close() (resultErr error) {
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(o.job)) }()
	if err := windows.TerminateJobObject(o.job, 1); err != nil {
		return err
	}
	// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, including ActiveProcesses.
	var accounting struct {
		user, kernel, periodUser, periodKernel int64
		faults, total, active, terminated      uint32
	}
	deadline := o.budget.until()
	for {
		if err := windows.QueryInformationJobObject(o.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return err
		}
		if accounting.active == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return errCommand
		}
		time.Sleep(time.Millisecond)
	}
}

func executable(info os.FileInfo) bool { return info.Mode().IsRegular() }
func environment() ([]string, error) {
	root, err := windows.GetWindowsDirectory()
	if err != nil {
		return nil, err
	}
	return []string{"SystemRoot=" + root, "WINDIR=" + root, "TEMP=" + os.TempDir(), "TMP=" + os.TempDir()}, nil
}

package dispatchlock

import (
	"errors"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"golang.org/x/sys/windows"
	"os"
	"strings"
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
	var data windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &data); err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, actual) || data.NumberOfLinks != 1 || data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", execution.ErrInvalid
	}
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(windows.Handle(f.Fd()), &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", err
	}
	if n >= uint32(len(buf)) {
		return "", execution.ErrInvalid
	}
	return strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`), nil
}
func tryLock(f *os.File) error {
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return execution.ErrBusy
	}
	return err
}
func unlock(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{})
}

func privateSidecar(_ *os.File, path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;SY)")
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

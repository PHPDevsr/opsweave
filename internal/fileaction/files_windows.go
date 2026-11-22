package fileaction

import (
	"errors"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(dir.Fd()), ObjectName: objectName, Attributes: windows.OBJ_DONT_REPARSE | windows.OBJ_CASE_INSENSITIVE}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ, &attributes, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		var nt windows.NTStatus
		if errors.As(err, &nt) {
			return nil, nt.Errno()
		}
		return nil, err
	}
	return os.NewFile(uintptr(handle), "filesystem-local"), nil
}

func unlinkRegular(_ *os.Root, _ string, file *os.File) (resultErr error) {
	// Reopen the validated regular file by handle, never by an absolute path.
	// Do not arm delete-on-close until its type and identity have been checked.
	library, err := windows.LoadLibraryEx("kernel32.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, windows.FreeLibrary(library)) }()
	proc, err := windows.GetProcAddress(library, "ReOpenFile")
	if err != nil {
		return err
	}
	value, _, callErr := syscall.SyscallN(proc, file.Fd(), windows.DELETE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_FLAG_OPEN_REPARSE_POINT)
	h := windows.Handle(value)
	if h == windows.InvalidHandle {
		return callErr
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(h)) }()
	flags := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS)
	return windows.SetFileInformationByHandle(h, windows.FileDispositionInfoEx, (*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)))
}

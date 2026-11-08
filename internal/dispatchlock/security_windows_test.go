package dispatchlock

import (
	"golang.org/x/sys/windows"
	"path/filepath"
	"testing"
	"unsafe"
)

func TestSidecarHasProtectedOwnerACL(t *testing.T) {
	lock, err := Acquire(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	descriptor, err := windows.GetNamedSecurityInfo(lock.Path()+".dispatcher.lock", windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("sidecar DACL is not protected", err)
	}
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil || acl.AceCount != 2 {
		t.Fatal("sidecar must grant exactly owner and SYSTEM", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	ownerSeen, systemSeen := false, false
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		// FILE_ALL_ACCESS, with no inherited/object-inheritance flags.
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || ace.Mask != 0x1f01ff {
			t.Fatalf("unexpected access grant: %+v", ace)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		ownerSeen = ownerSeen || sid.Equals(user.User.Sid)
		systemSeen = systemSeen || sid.Equals(system)
		if !sid.Equals(user.User.Sid) && !sid.Equals(system) {
			t.Fatal("unexpected sidecar principal", sid.String())
		}
	}
	if !ownerSeen || !systemSeen {
		t.Fatal("missing owner or SYSTEM grant")
	}
}

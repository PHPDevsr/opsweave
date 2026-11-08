package dispatchlock

import (
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"testing"
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
	text := descriptor.String()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "D:P") || strings.Count(text, "(A;") != 2 || !strings.Contains(text, user.User.Sid.String()) || !strings.Contains(text, ";;;SY)") {
		t.Fatalf("sidecar ACL not protected to owner/SYSTEM: %s", text)
	}
}

//go:build linux || darwin

package dispatchlock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSidecarPermissionsArePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	if err := os.WriteFile(path+".dispatcher.lock", nil, 0666); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	info, err := os.Stat(lock.Path() + ".dispatcher.lock")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
}

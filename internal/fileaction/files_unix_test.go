//go:build linux || darwin

package fileaction

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUnixSpecialFileAndNonblockingOpen(t *testing.T) {
	r, src, _ := fixture(t)
	name := filepath.Join(src, "fifo")
	if err := syscall.Mkfifo(name, 0600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"file.exists", "file.cleanup", "file.copy"} {
		in := map[string]any{"root": "source", "path": "fifo"}
		if id == "file.copy" {
			in = copyInput("fifo", "blocked")
		}
		if out, err := r.Invoke(t.Context(), id, "1", in); err == nil || len(out) != 0 {
			t.Fatal(id, out, err)
		}
	}
	root, err := os.OpenRoot(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	done := make(chan error, 1)
	go func() {
		f, err := openNoFollow(root, "fifo")
		if err == nil {
			err = f.Close()
		}
		done <- err
		close(done)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		// Rescue a wrongly blocking reader so the failing test still joins it.
		writer, err := syscall.Open(name, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			if err := syscall.Close(writer); err != nil {
				t.Error(err)
			}
		} else if !errors.Is(err, syscall.ENXIO) {
			t.Error(err)
		}
		<-done
		t.Fatal("native read flags blocked on a FIFO")
	}
}

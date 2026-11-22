//go:build linux || darwin

// Package localipc supplies isolated local IPC listeners only for tests/benchmarks.
package localipc

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// Listen creates a short, private socket path so macOS's sockaddr limit is respected.
func Listen(t testing.TB) (net.Listener, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ow-ipc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	p := filepath.Join(dir, "daemon.sock")
	l, err := net.Listen("unix", p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil && !isClosed(err) {
			t.Error(err)
		}
	})
	return l, "unix://" + p
}

//go:build linux || darwin

package shellaction

import (
	"context"
	"net"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestUnixOwnedProcessGroup(t *testing.T) {
	p := fixture(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Executable, "hold", listener.Addr().String())
	cmd.Env, err = environment()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := prepare(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		if cmd.Process != nil && cmd.ProcessState == nil {
			if err := cmd.Wait(); err == nil {
				t.Error("expected cancelled test process")
			}
		}
		if err := owner.close(); err != nil {
			t.Error(err)
		}
	})
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	group, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil || group != cmd.Process.Pid {
		t.Error(group, err)
	}
	cancel()
	if err := cmd.Wait(); err == nil {
		t.Error("owned root was not stopped")
	}
}

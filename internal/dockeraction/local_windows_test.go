package dockeraction

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/PHPDevsr/opsweave/internal/testutil/localipc"
	"golang.org/x/sys/windows"
)

func TestWindowsBusyPipeDialRespectsDeadline(t *testing.T) {
	_, endpoint := localipc.Listen(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	first, err := dialLocal(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	}()
	busy, cancelBusy := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelBusy()
	second, err := dialLocal(busy, endpoint)
	if err == nil {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("busy pipe unexpectedly connected")
	}
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatal("busy pipe ignored its connection budget", err)
	}
}

func TestWindowsPipeCannotImpersonateServiceToken(t *testing.T) {
	endpoint := localipc.Serve(t, func(conn net.Conn) {
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			t.Error(err)
			return
		}
		file, ok := conn.(interface{ Fd() uintptr })
		if !ok {
			t.Error("fixture is not native pipe")
			return
		}
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		proc := windows.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateNamedPipeClient")
		result, _, callErr := proc.Call(file.Fd())
		if result == 0 {
			if !errors.Is(callErr, windows.ERROR_CANNOT_IMPERSONATE) {
				t.Error(callErr)
			}
		} else {
			// Always restore the thread before it returns to the Go runtime.
			defer func() {
				if err := windows.RevertToSelf(); err != nil {
					t.Error(err)
				}
			}()
			var token windows.Token
			err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token)
			if err == nil {
				var level, size uint32
				err = windows.GetTokenInformation(token, windows.TokenImpersonationLevel, (*byte)(unsafe.Pointer(&level)), uint32(unsafe.Sizeof(level)), &size)
				if err != nil || level != windows.SecurityAnonymous {
					t.Error("pipe obtained non-anonymous token", level, err)
				}
				if err := token.Close(); err != nil {
					t.Error(err)
				}
			} else if !errors.Is(err, windows.ERROR_CANT_OPEN_ANONYMOUS) {
				t.Error(err)
			}
		}
		if _, err := io.WriteString(conn, response(200, inspectJSON())); err != nil {
			t.Error(err)
		}
	})
	if _, err := runtimeFor(t, endpoint, "docker.inspect").Invoke(t.Context(), "docker.inspect", "1", map[string]any{"container": containerID}); err != nil {
		t.Fatal(err)
	}
}

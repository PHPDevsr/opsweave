package dockeraction

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

type pipeConn struct{ *os.File }

func (*pipeConn) LocalAddr() net.Addr  { return localAddr{} }
func (*pipeConn) RemoteAddr() net.Addr { return localAddr{} }

func dialLocal(ctx context.Context, endpoint string) (net.Conn, error) {
	name, err := windows.UTF16PtrFromString(`\\.\pipe\` + strings.TrimPrefix(endpoint, "npipe:////./pipe/"))
	if err != nil {
		return nil, errAction
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Anonymous SQOS prevents an IPC server from impersonating our token.
		h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_ANONYMOUS, 0)
		if err == nil {
			f := os.NewFile(uintptr(h), "docker-local")
			deadline, _ := ctx.Deadline()
			if err := f.SetDeadline(deadline); err != nil {
				return nil, errors.Join(err, f.Close())
			}
			return &pipeConn{f}, nil
		}
		if !errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return nil, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

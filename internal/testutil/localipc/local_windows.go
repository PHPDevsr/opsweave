package localipc

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type pipeConn struct{ *os.File }

func (*pipeConn) LocalAddr() net.Addr  { return address{} }
func (*pipeConn) RemoteAddr() net.Addr { return address{} }

type listener struct {
	mu       sync.Mutex
	name     *uint16
	security *windows.SECURITY_DESCRIPTOR
	pending  windows.Handle
	closed   chan struct{}
	once     sync.Once
	closeErr error
}

// Listen creates a random, current-user/SYSTEM-only byte pipe, rejecting remote clients.
func Listen(t testing.TB) (net.Listener, string) {
	t.Helper()
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	name := "opsweave-test-" + hex.EncodeToString(random[:])
	p, err := windows.UTF16PtrFromString(`\\.\pipe\` + name)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")(A;;GA;;;SY)")
	if err != nil {
		t.Fatal(err)
	}
	l := &listener{name: p, security: sd, closed: make(chan struct{}), pending: windows.InvalidHandle}
	l.pending, err = l.create()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	})
	return l, "npipe:////./pipe/" + name
}

func (l *listener) create() (windows.Handle, error) {
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: l.security}
	return windows.CreateNamedPipe(l.name, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED, windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS, windows.PIPE_UNLIMITED_INSTANCES, 4096, 4096, 0, &sa)
}

func (l *listener) Accept() (net.Conn, error) {
	l.mu.Lock()
	select {
	case <-l.closed:
		l.mu.Unlock()
		return nil, net.ErrClosed
	default:
	}
	h := l.pending
	l.pending = windows.InvalidHandle
	l.mu.Unlock()
	if err := l.connect(h); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(h))
	}
	next, err := l.create()
	if err != nil {
		return nil, errors.Join(err, windows.CloseHandle(h))
	}
	l.mu.Lock()
	select {
	case <-l.closed:
		l.mu.Unlock()
		return nil, errors.Join(net.ErrClosed, windows.CloseHandle(next), windows.CloseHandle(h))
	default:
		l.pending = next
		l.mu.Unlock()
	}
	f := os.NewFile(uintptr(h), "test-local")
	if err := f.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return &pipeConn{f}, nil
}

func (l *listener) connect(h windows.Handle) (resultErr error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(event)) }()
	o := windows.Overlapped{HEvent: event}
	// Windows retains this address after ConnectNamedPipe returns. Pin it
	// until cancellation/completion has been joined; a Go stack may move.
	var pinned runtime.Pinner
	pinned.Pin(&o)
	defer pinned.Unpin()
	err = windows.ConnectNamedPipe(h, &o)
	if errors.Is(err, windows.ERROR_PIPE_CONNECTED) || err == nil {
		return nil
	}
	if !errors.Is(err, windows.ERROR_IO_PENDING) {
		return err
	}
	var transferred uint32
	// Join pending kernel I/O on every exit, including a failed event wait.
	defer func() {
		cancelErr := windows.CancelIoEx(h, &o)
		if errors.Is(cancelErr, windows.ERROR_NOT_FOUND) {
			cancelErr = nil
		}
		joinErr := windows.GetOverlappedResult(h, &o, &transferred, true)
		if errors.Is(joinErr, windows.ERROR_OPERATION_ABORTED) {
			joinErr = nil
		}
		resultErr = errors.Join(resultErr, cancelErr, joinErr)
	}()
	for {
		select {
		case <-l.closed:
			return net.ErrClosed
		default:
		}
		status, err := windows.WaitForSingleObject(event, 10)
		if err != nil {
			return err
		}
		if status == windows.WAIT_OBJECT_0 {
			return windows.GetOverlappedResult(h, &o, &transferred, true)
		}
	}
}

func (l *listener) Close() error {
	l.once.Do(func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		close(l.closed)
		if l.pending != windows.InvalidHandle {
			l.closeErr = windows.CloseHandle(l.pending)
			l.pending = windows.InvalidHandle
		}
	})
	return l.closeErr
}
func (*listener) Addr() net.Addr { return address{} }

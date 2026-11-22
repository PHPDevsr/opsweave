package localipc

import (
	"errors"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Serve has one bounded connection owner, joined even when a test fails.
func Serve(t testing.TB, serve func(net.Conn)) string {
	t.Helper()
	l, endpoint := Listen(t)
	var mu sync.Mutex
	var active net.Conn
	stopping := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := l.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					t.Error(err)
				}
				return
			}
			mu.Lock()
			if stopping {
				mu.Unlock()
				_ = conn.Close()
				return
			}
			active = conn
			mu.Unlock()
			if err := conn.SetDeadline(time.Now().Add(8 * time.Second)); err != nil {
				t.Error(err)
			}
			serve(conn)
			if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrClosed) {
				t.Error(err)
			}
			mu.Lock()
			active = nil
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		mu.Lock()
		stopping = true
		conn := active
		mu.Unlock()
		if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
		if conn != nil {
			if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrClosed) {
				t.Error(err)
			}
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			stack := make([]byte, 65536)
			count := runtime.Stack(stack, true)
			t.Errorf("fake daemon did not join: %s", stack[:count])
		}
	})
	return endpoint
}

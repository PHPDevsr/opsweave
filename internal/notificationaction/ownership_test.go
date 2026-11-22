package notificationaction

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

type closeFailure struct {
	net.Conn
	calls atomic.Int32
}

func (c *closeFailure) Close() error {
	c.calls.Add(1)
	return errors.Join(c.Conn.Close(), errors.New("synthetic-secret-close-error"))
}

func TestAcknowledgedHTTPWithCleanupFailure(t *testing.T) {
	server, deps := httpFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	dial := deps.Dial
	var conn *closeFailure
	deps.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		raw, err := dial(ctx, network, address)
		if err != nil {
			return raw, err
		}
		conn = &closeFailure{Conn: raw}
		return conn, nil
	}
	deps.LookupEnv = func(string) (string, bool) { return server.URL, true }
	out, err := runtimeFor(t, webhookProfile(server.URL), deps).Invoke(t.Context(), "notification.webhook", "1", map[string]any{"profile": "ops", "message": "hi"})
	if !errors.Is(err, execution.ErrAction) || len(out) != 0 || strings.Contains(err.Error(), "synthetic-secret") || conn.calls.Load() != 1 {
		t.Fatalf("acknowledged cleanup failure: %s %v", out, err)
	}
}

func TestRejectedHTTPBodyClosesBeforeDrain(t *testing.T) {
	peerClosed := make(chan struct{})
	server, deps := httpFixture(t, func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		defer close(peerClosed)
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 4194304\r\n\r\n"+strings.Repeat("x", 8193)); err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, conn)
	})
	deps.LookupEnv = func(string) (string, bool) { return server.URL, true }
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	out, err := runtimeFor(t, webhookProfile(server.URL), deps).Invoke(ctx, "notification.webhook", "1", map[string]any{"profile": "ops", "message": "hi"})
	if !errors.Is(err, execution.ErrAction) || len(out) != 0 || ctx.Err() != nil {
		t.Fatalf("body cleanup waited for deadline: %s %v", out, err)
	}
	select {
	case <-peerClosed:
	case <-ctx.Done():
		t.Fatal("rejected response socket leaked")
	}
}

func TestTLSHostnameAndPrivateGrantEnforcement(t *testing.T) {
	var hits atomic.Int32
	server, deps := httpFixture(t, func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(204) })
	deps.LookupEnv = func(string) (string, bool) { return "https://example.com/secret", true }
	if _, err := runtimeFor(t, webhookProfile("https://example.com"), deps).Invoke(t.Context(), "notification.webhook", "1", map[string]any{"profile": "ops", "message": "hi"}); err == nil {
		t.Fatal("wrong TLS hostname allowed")
	}
	p := webhookProfile(server.URL)
	p.AllowedPrivateCIDRs = nil
	deps.Dial = nil
	deps.LookupEnv = func(string) (string, bool) { return server.URL, true }
	if _, err := runtimeFor(t, p, deps).Invoke(t.Context(), p.Action, "1", map[string]any{"profile": "ops", "message": "hi"}); err == nil || hits.Load() != 0 {
		t.Fatal("ungranted private endpoint reached")
	}
}

func TestDialAndDeadlineFailures(t *testing.T) {
	for _, withConn := range []bool{false, true} {
		t.Run(map[bool]string{false: "nil-connection", true: "connection-with-error"}[withConn], func(t *testing.T) {
			var conn *closeFailure
			deps := Dependencies{LookupEnv: func(string) (string, bool) { return "https://example.com/secret", true }, Dial: func(context.Context, string, string) (net.Conn, error) {
				if !withConn {
					return nil, nil
				}
				left, right := net.Pipe()
				if err := right.Close(); err != nil {
					t.Fatal(err)
				}
				conn = &closeFailure{Conn: left}
				return conn, errors.New("secret-dial-error")
			}}
			out, err := runtimeFor(t, webhookProfile("https://example.com"), deps).Invoke(t.Context(), "notification.webhook", "1", map[string]any{"profile": "ops", "message": "hi"})
			if !errors.Is(err, execution.ErrAction) || len(out) != 0 || conn != nil && conn.calls.Load() != 1 {
				t.Fatalf("%s %v", out, err)
			}
		})
	}
}

func TestEmailCancellationJoinsTLSAndGreeting(t *testing.T) {
	for _, stage := range []string{"handshake", "greeting"} {
		t.Run(stage, func(t *testing.T) {
			cert, pool := certificate(t)
			left, right := net.Pipe()
			defer right.Close()
			ready := make(chan struct{})
			peerDone := make(chan struct{})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			go func() {
				defer close(peerDone)
				defer right.Close()
				_ = right.SetDeadline(time.Now().Add(5 * time.Second))
				if stage == "handshake" {
					var first [1]byte
					if _, err := io.ReadFull(right, first[:]); err != nil {
						return
					}
					close(ready)
					_, _ = io.Copy(io.Discard, right)
					return
				}
				wire := tls.Server(right, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
				if wire.Handshake() != nil {
					return
				}
				close(ready)
				_, _ = io.Copy(io.Discard, wire)
			}()
			p := Profile{Name: "ops", Action: "notification.email", Host: "localhost", Port: 465, TLSMode: "implicit_tls", UsernameEnv: testURL, PasswordEnv: testToken, From: "from@example.com", Recipients: []string{"to@example.com"}}
			deps := Dependencies{RootCAs: pool, LookupEnv: func(string) (string, bool) { return "synthetic", true }, Dial: func(context.Context, string, string) (net.Conn, error) { return left, nil }}
			runtime := runtimeFor(t, p, deps)
			done := make(chan error, 1)
			go func() {
				_, err := runtime.Invoke(ctx, p.Action, "1", map[string]any{"profile": "ops", "subject": "test", "message": "hi"})
				done <- err
			}()
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal("fixture not reached")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("SMTP invocation leaked")
			}
			select {
			case <-peerDone:
			case <-time.After(3 * time.Second):
				t.Fatal("SMTP peer leaked")
			}
		})
	}
}

func TestMailEncodingAndReferenceBoundaries(t *testing.T) {
	p := Profile{From: "from@example.com", Recipients: []string{"to@example.com"}}
	data := emailMessage(p, strings.Repeat("世", 85), strings.Repeat("世", 1365))
	if len(data) > 32<<10 {
		t.Fatal("maximum inputs exceed outbound budget")
	}
	for _, line := range strings.Split(strings.SplitN(data, "\r\n\r\n", 2)[1], "\r\n") {
		if len(line) > 76 {
			t.Fatal("base64 line too long")
		}
	}
	if printable("secret\u0085value", 4096) || mailbox("世界@example.com") || validName("") || validName(strings.Repeat("a", 65)) {
		t.Fatal("invalid boundary accepted")
	}
	names := (Config{Profiles: []Profile{{TokenEnv: testToken, URLEnv: testURL}, {PasswordEnv: testToken}}}).SecretNames()
	if len(names) != 2 || names[0] != testToken || names[1] != testURL {
		t.Fatal(names)
	}
	raw := &receiveConn{remaining: 0}
	if n, err := raw.Read(nil); n != 0 || err != nil {
		t.Fatal("empty Read invalid")
	}
	if _, err := raw.Read(make([]byte, 1)); err == nil {
		t.Fatal("receive budget ignored")
	}
}

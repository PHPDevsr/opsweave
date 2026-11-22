package notificationaction

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

const testURL = "OPSWEAVE_ACTION_SECRET_URL"
const testToken = "OPSWEAVE_ACTION_SECRET_TOKEN"

func certificate(t testing.TB) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "notification fixture"}, DNSNames: []string{"api.telegram.org", "hooks.slack.com", "localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func runtimeFor(t testing.TB, p Profile, deps Dependencies) *action.Runtime {
	t.Helper()
	entries, err := Registrations(Config{Enabled: true, Profiles: []Profile{p}}, deps)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := action.New(entries, []string{p.Action})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func httpFixture(t testing.TB, handler http.HandlerFunc) (*httptest.Server, Dependencies) {
	t.Helper()
	cert, pool := certificate(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, Dependencies{RootCAs: pool, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
}

func TestHTTPChannels(t *testing.T) {
	for _, id := range []string{"notification.telegram", "notification.slack", "notification.webhook"} {
		t.Run(id, func(t *testing.T) {
			var count atomic.Int32
			server, deps := httpFixture(t, func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || !r.Close {
					t.Error("incorrect request envelope")
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				switch id {
				case "notification.telegram":
					if r.Host != "api.telegram.org:443" || r.URL.Path != "/bot123:synthetic-token/sendMessage" || payload["chat_id"] != "-12" || payload["text"] != "hello\n世界" || len(payload) != 2 {
						t.Error("incorrect Telegram payload")
					}
					io.WriteString(w, `{"ok":true,"result":{"private":"ignored"}}`)
				case "notification.slack":
					if r.URL.Path != "/services/T123/B123/synthetic" || len(payload) != 3 || payload["text"] != "hello\n世界" || payload["unfurl_links"] != false || payload["unfurl_media"] != false {
						t.Error("incorrect Slack payload")
					}
					io.WriteString(w, "ok")
				default:
					if payload["message"] != "hello\n世界" || len(payload) != 1 || r.Header.Get("Authorization") != "Bearer synthetic-bearer" {
						t.Error("incorrect webhook payload")
					}
					w.WriteHeader(204)
				}
			})
			p := Profile{Name: "ops", Action: id}
			secrets := map[string]string{}
			switch id {
			case "notification.telegram":
				p.ChatID = "-12"
				p.TokenEnv = testToken
				secrets[testToken] = "123:synthetic-token"
			case "notification.slack":
				p.URLEnv = testURL
				secrets[testURL] = "https://hooks.slack.com/services/T123/B123/synthetic"
			default:
				p.URLEnv = testURL
				p.BearerEnv = testToken
				p.AllowedOrigin = server.URL
				p.AllowedPrivateCIDRs = []string{"127.0.0.1/32"}
				secrets[testURL] = server.URL + "/fixed?key=synthetic"
				secrets[testToken] = "synthetic-bearer"
				deps.Dial = nil
			}
			deps.LookupEnv = func(k string) (string, bool) { v, ok := secrets[k]; return v, ok }
			runtime := runtimeFor(t, p, deps)
			// Credentials are an immutable runtime snapshot, not read again on invocation.
			clear(secrets)
			p.AllowedPrivateCIDRs = nil
			out, err := runtime.Invoke(t.Context(), id, "1", map[string]any{"profile": "ops", "message": "hello\n世界"})
			if err != nil || string(out) != `{"accepted":true}` || count.Load() != 1 {
				t.Fatalf("output=%s error=%v attempts=%d", out, err, count.Load())
			}
			catalog, err := runtime.Catalog(map[string]string{id: "1"})
			data, _ := json.Marshal(catalog)
			if err != nil || strings.Contains(string(data), "synthetic") {
				t.Fatalf("catalog leaked credentials: %v", err)
			}
		})
	}
}

func webhookProfile(origin string) Profile {
	return Profile{Name: "ops", Action: "notification.webhook", URLEnv: testURL, AllowedOrigin: origin, AllowedPrivateCIDRs: []string{"127.0.0.1/32"}}
}

func TestHTTPRejectsResponsesAndTLS(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		trust     bool
	}{
		{"redirect", "HTTP/1.1 302 Found\r\nLocation: https://example.com/\r\nContent-Length: 0\r\n\r\n", true},
		{"error", "HTTP/1.1 500 Error\r\nContent-Length: 0\r\n\r\n", true},
		{"truncated", "HTTP/1.1 200 OK\r\nContent-Length: 9\r\n\r\nshort", true},
		{"oversized-body", "HTTP/1.1 200 OK\r\nContent-Length: 8193\r\n\r\n" + strings.Repeat("x", 8193), true},
		{"oversized-header", "HTTP/1.1 200 OK\r\nX-Large: " + strings.Repeat("x", 17000) + "\r\nContent-Length: 0\r\n\r\n", true},
		{"compressed", "HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: 0\r\n\r\n", true},
		{"invalid-utf8", "HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\n\xff", true},
		{"untrusted-certificate", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server, deps := httpFixture(t, func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				_, _ = io.WriteString(conn, tc.raw)
			})
			deps.LookupEnv = func(string) (string, bool) { return server.URL + "/synthetic-secret", true }
			if !tc.trust {
				deps.RootCAs = x509.NewCertPool()
			}
			out, err := runtimeFor(t, webhookProfile(server.URL), deps).Invoke(t.Context(), "notification.webhook", "1", map[string]any{"profile": "ops", "message": "hello"})
			if !errors.Is(err, execution.ErrAction) || len(out) != 0 || attempts.Load() > 1 {
				t.Fatalf("out=%s err=%v attempts=%d", out, err, attempts.Load())
			}
		})
	}
}

func TestProviderAcknowledgments(t *testing.T) {
	for _, id := range []string{"notification.telegram", "notification.slack"} {
		for _, body := range []string{`{"ok":false}`, `{"ok":null}`, `{}`, `{"ok":true}garbage`, "not-ok"} {
			t.Run(id+body, func(t *testing.T) {
				_, deps := httpFixture(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
				p := Profile{Name: "ops", Action: id, URLEnv: testURL}
				secret := "https://hooks.slack.com/services/T/B/secret"
				if id == "notification.telegram" {
					p.URLEnv = ""
					p.ChatID = "1"
					p.TokenEnv = testToken
					secret = "123:synthetic"
				}
				deps.LookupEnv = func(string) (string, bool) { return secret, true }
				out, err := runtimeFor(t, p, deps).Invoke(t.Context(), id, "1", map[string]any{"profile": "ops", "message": "hello"})
				if err == nil || len(out) != 0 {
					t.Fatalf("accepted bad acknowledgment: %s %v", out, err)
				}
			})
		}
	}
}

func TestHTTPTimeoutAndCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(strconv.FormatBool(deadline), func(t *testing.T) {
			ready := make(chan struct{})
			closed := make(chan struct{})
			server, deps := httpFixture(t, func(w http.ResponseWriter, r *http.Request) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				defer close(closed)
				close(ready)
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				_, _ = io.Copy(io.Discard, conn)
			})
			deps.LookupEnv = func(string) (string, bool) { return server.URL, true }
			runtime := runtimeFor(t, webhookProfile(server.URL), deps)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := context.Canceled
			if deadline {
				ctx, cancel = context.WithTimeout(t.Context(), 250*time.Millisecond)
				defer cancel()
				want = context.DeadlineExceeded
			}
			done := make(chan error, 1)
			go func() {
				out, err := runtime.Invoke(ctx, "notification.webhook", "1", map[string]any{"profile": "ops", "message": "hi"})
				if len(out) != 0 {
					done <- errors.New("unexpected output")
					return
				}
				done <- err
			}()
			select {
			case <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("request not received")
			}
			if !deadline {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("invocation leaked")
			}
			select {
			case <-closed:
			case <-time.After(5 * time.Second):
				t.Fatal("peer connection leaked")
			}
		})
	}
}

type smtpFixtureOptions struct{ mode, fail string }

func smtpFixture(t *testing.T, opts smtpFixtureOptions) (Profile, Dependencies, <-chan string) {
	t.Helper()
	cert, pool := certificate(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	captured := make(chan string, 1)
	// The single accepted connection has a bounded deadline and is joined at cleanup.
	go func() {
		defer close(done)
		raw, err := listener.Accept()
		if err != nil {
			return
		}
		defer raw.Close()
		_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
		var conn net.Conn = raw
		secure := opts.mode == "implicit_tls"
		if secure {
			wire := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
			if wire.Handshake() != nil {
				return
			}
			conn = wire
		}
		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		reply := func(s string) bool {
			_, err := rw.WriteString(s + "\r\n")
			if err != nil {
				return false
			}
			return rw.Flush() == nil
		}
		if opts.fail == "greeting-size" {
			reply("220 " + strings.Repeat("x", 270000))
			return
		}
		if !reply("220 localhost fixture") {
			return
		}
		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "EHLO "):
				if !secure && opts.fail != "no-starttls" {
					if !reply("250-localhost\r\n250-STARTTLS\r\n250 AUTH PLAIN") {
						return
					}
				} else {
					methods := "PLAIN"
					if opts.fail == "no-plain" {
						methods = "LOGIN"
					}
					if !reply("250-localhost\r\n250 AUTH " + methods) {
						return
					}
				}
			case line == "STARTTLS":
				if !reply("220 upgrade") {
					return
				}
				wire := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
				if wire.Handshake() != nil {
					return
				}
				conn = wire
				rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
				secure = true
			case strings.HasPrefix(line, "AUTH PLAIN "):
				decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "AUTH PLAIN "))
				if !secure || err != nil || string(decoded) != "\x00synthetic-user\x00synthetic-pass" {
					t.Error("credentials or TLS boundary incorrect")
					return
				}
				if opts.fail == "auth" {
					reply("535 denied")
					return
				}
				if !reply("235 authenticated") {
					return
				}
			case strings.HasPrefix(line, "MAIL FROM:"):
				if line != "MAIL FROM:<from@example.com>" {
					t.Error("unexpected sender")
				}
				if opts.fail == "sender" {
					reply("550 denied")
					return
				}
				if !reply("250 sender ok") {
					return
				}
			case strings.HasPrefix(line, "RCPT TO:"):
				if line != "RCPT TO:<to@example.com>" {
					t.Error("unexpected recipient")
				}
				if opts.fail == "recipient" {
					reply("550 denied")
					return
				}
				if !reply("250 recipient ok") {
					return
				}
			case line == "DATA":
				if opts.fail == "data" {
					reply("554 denied")
					return
				}
				if !reply("354 send body") {
					return
				}
				data, err := textproto.NewReader(rw.Reader).ReadDotBytes()
				if err != nil {
					return
				}
				captured <- string(data)
				if opts.fail == "ack" {
					return
				}
				if !reply("250 accepted") {
					return
				}
			case line == "QUIT":
				if opts.fail == "quit" {
					reply("500 failure")
					return
				}
				reply("221 bye")
				return
			default:
				t.Errorf("unexpected SMTP command %q", strings.SplitN(line, " ", 2)[0])
				return
			}
		}
	}()
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("SMTP fixture leaked")
		}
	})
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	p := Profile{Name: "ops", Action: "notification.email", Host: "127.0.0.1", Port: number, TLSMode: opts.mode, UsernameEnv: testURL, PasswordEnv: testToken, From: "from@example.com", Recipients: []string{"to@example.com"}, AllowedPrivateCIDRs: []string{"127.0.0.1/32"}}
	deps := Dependencies{RootCAs: pool, LookupEnv: func(k string) (string, bool) {
		if k == testURL {
			return "synthetic-user", true
		}
		return "synthetic-pass", true
	}}
	return p, deps, captured
}

func TestEmailTLSDeliveryAndMIME(t *testing.T) {
	for _, mode := range []string{"implicit_tls", "starttls"} {
		t.Run(mode, func(t *testing.T) {
			p, deps, captured := smtpFixture(t, smtpFixtureOptions{mode: mode})
			out, err := runtimeFor(t, p, deps).Invoke(t.Context(), p.Action, "1", map[string]any{"profile": "ops", "message": "hello\r\n.世界", "subject": "Status 世界"})
			if err != nil || string(out) != `{"accepted":true}` {
				t.Fatalf("%s %v", out, err)
			}
			select {
			case raw := <-captured:
				message, err := mail.ReadMessage(strings.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
				if err != nil || subject != "Status 世界" {
					t.Fatal("subject encoding")
				}
				data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, message.Body))
				if err != nil || string(data) != "hello\r\n.世界" {
					t.Fatalf("body %q %v", data, err)
				}
				if strings.Contains(raw, "synthetic-") || message.Header.Get("To") != "<to@example.com>" || message.Header.Get("Content-Type") != "text/plain; charset=UTF-8" {
					t.Fatal("MIME boundary")
				}
			default:
				t.Fatal("missing DATA")
			}
		})
	}
}

func TestEmailFailures(t *testing.T) {
	for _, failure := range []string{"no-starttls", "no-plain", "auth", "sender", "recipient", "data", "ack", "quit", "greeting-size", "certificate", "starttls-certificate"} {
		t.Run(failure, func(t *testing.T) {
			mode := "implicit_tls"
			if failure == "no-starttls" || failure == "starttls-certificate" {
				mode = "starttls"
			}
			p, deps, _ := smtpFixture(t, smtpFixtureOptions{mode: mode, fail: failure})
			if failure == "certificate" || failure == "starttls-certificate" {
				deps.RootCAs = x509.NewCertPool()
			}
			out, err := runtimeFor(t, p, deps).Invoke(t.Context(), p.Action, "1", map[string]any{"profile": "ops", "message": "hello", "subject": "test"})
			if !errors.Is(err, execution.ErrAction) || len(out) != 0 {
				t.Fatalf("%s %v", out, err)
			}
		})
	}
}

func BenchmarkNotificationWebhook(b *testing.B) {
	server, deps := httpFixture(b, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			b.Error(err)
		}
		w.WriteHeader(204)
	})
	deps.LookupEnv = func(string) (string, bool) { return server.URL, true }
	deps.Dial = nil
	p := webhookProfile(server.URL)
	runtime := runtimeFor(b, p, deps)
	input := map[string]any{"profile": "ops", "message": "synthetic benchmark"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := runtime.Invoke(b.Context(), p.Action, "1", input); err != nil {
			b.Fatal(err)
		}
	}
}

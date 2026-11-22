package httpaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

func testHandler(t testing.TB, origin string) handler {
	t.Helper()
	p, err := newPolicy(Config{Enabled: true, AllowedOrigins: []string{origin}, AllowedPrivateCIDRs: []string{"127.0.0.1/32"}})
	if err != nil {
		t.Fatal(err)
	}
	return handler{policy: p, lookup: net.DefaultResolver.LookupNetIP, dial: (&net.Dialer{}).DialContext}
}
func runtimeFor(t testing.TB, h handler) *action.Runtime {
	t.Helper()
	r, err := action.New(h.registrations(), []string{"http.request", "http.health_check"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestPolicyValidation(t *testing.T) {
	for _, c := range []Config{
		{Enabled: true}, {AllowedOrigins: []string{"https://user:secret-marker@host"}},
		{AllowedOrigins: []string{"https://example.com/"}}, {AllowedOrigins: []string{"https://example.com?"}},
		{AllowedOrigins: []string{"https://example.com", "https://EXAMPLE.com:443"}},
		{AllowedPrivateCIDRs: []string{"0.0.0.0/0"}}, {AllowedPrivateCIDRs: []string{"169.254.0.0/16"}},
		{AllowedPrivateCIDRs: []string{"127.0.0.1/8"}}, {AllowedPrivateCIDRs: []string{"10.0.0.0/8", "10.0.0.0/8"}},
		{AllowedPrivateCIDRs: []string{"::ffff:127.0.0.0/104"}}, {AllowedOrigins: make([]string, 129)},
	} {
		if err := c.Validate(); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatalf("unsafe validation: %v", err)
		}
	}
	r, cat, err := Runtime(Config{})
	if err != nil || len(cat) != 0 || !errors.Is(r.Check("http.request", "1"), execution.ErrUnavailable) {
		t.Fatal(cat, err)
	}
	c := Config{Enabled: true, AllowedOrigins: []string{"https://example.com"}}
	r, cat, err = Runtime(c)
	if err != nil || len(cat) != 2 {
		t.Fatal(cat, err)
	}
	c.AllowedOrigins[0] = "https://other.example" // Composition owns a policy snapshot.
	if r.Check("http.request", "2") != execution.ErrUnavailable {
		t.Fatal("version not pinned")
	}
	h := testHandler(t, "https://example.com")
	denied, err := action.New(h.registrations(), nil)
	if err != nil || denied.Check("http.request", "1") != execution.ErrPermission {
		t.Fatal(err)
	}
}
func TestURLPolicy(t *testing.T) {
	for _, raw := range []string{"file:///tmp/a", "http://x#", "http://x/#secret-marker", "http://x:0", "http://x:65536", "http://x:", "http://[::ffff:127.0.0.1]", "http://[fe80::1%25eth0]", "http://a\\b", "http://x. /", "http://x.", "http://-x", "http://x..y", "http://x/\n", "http://user:secret-marker@x", "http://é.example", strings.Repeat("x", 8193)} {
		if _, _, err := parseURL(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	for _, raw := range []string{"http://example.com/path?q=ok", "https://EXAMPLE.com:443/path", "http://[::1]:80/path"} {
		if _, _, err := parseURL(raw); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}
func TestIPPolicy(t *testing.T) {
	p, err := newPolicy(Config{AllowedPrivateCIDRs: []string{"127.0.0.1/32", "10.1.0.0/16", "fc00::/7", "::1/128"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"0.0.0.0", "0.1.2.3", "127.0.0.2", "10.2.0.1", "169.254.169.254", "100.100.100.200", "168.63.129.16", "192.0.0.8", "192.0.2.1", "192.88.99.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "255.255.255.255", "::", "::ffff:8.8.8.8", "fe80::1", "ff02::1", "64:ff9b::a00:1", "2001::1", "2001:db8::1", "2002::1", "3fff::1", "fd00:ec2::254"} {
		if p.allowsIP(netip.MustParseAddr(raw)) {
			t.Errorf("allowed %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111", "127.0.0.1", "10.1.0.1", "fd00::1", "::1"} {
		if !p.allowsIP(netip.MustParseAddr(raw)) {
			t.Errorf("denied %s", raw)
		}
	}
}
func TestDNSPinningRebindingAndCancellation(t *testing.T) {
	h := testHandler(t, "http://example.com")
	answers := []netip.Addr{netip.MustParseAddr("8.8.8.8")}
	h.lookup = func(context.Context, string, string) ([]netip.Addr, error) { return answers, nil }
	var dialed []string
	h.dial = func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed = append(dialed, addr)
		return nil, errors.New("secret-marker")
	}
	if _, err := h.connect(t.Context(), "tcp", "example.com:80"); err == nil || strings.Contains(err.Error(), "secret-marker") {
		t.Fatal(err)
	}
	if len(dialed) != 1 || dialed[0] != "8.8.8.8:80" {
		t.Fatal(dialed)
	}
	answers = append(answers, netip.MustParseAddr("169.254.169.254"))
	if _, err := h.connect(t.Context(), "tcp", "example.com:80"); err == nil || len(dialed) != 1 {
		t.Fatal("mixed answer reached dialer")
	}
	answers = []netip.Addr{netip.MustParseAddr("127.0.0.2")}
	if _, err := h.connect(t.Context(), "tcp", "example.com:80"); err == nil || len(dialed) != 1 {
		t.Fatal("rebinding reached dialer")
	}
	h.lookup = func(ctx context.Context, _, _ string) ([]netip.Addr, error) { <-ctx.Done(); return nil, ctx.Err() }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.connect(ctx, "tcp", "example.com:80"); err == nil {
		t.Fatal("canceled DNS accepted")
	}
}
func TestHTTPBehaviorAndBounds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/redirect":
			w.Header().Set("Location", "http://169.254.169.254/")
			w.WriteHeader(302)
		case "/error":
			w.WriteHeader(503)
			fmt.Fprint(w, "secret-marker")
		case "/large":
			fmt.Fprint(w, strings.Repeat("a", 8193))
		case "/exact":
			fmt.Fprint(w, strings.Repeat("a", 8192))
		case "/binary":
			w.Write([]byte{0xff})
		case "/compressed":
			w.Header().Set("Content-Encoding", "gzip")
			fmt.Fprint(w, "data")
		case "/huge-headers":
			w.Header().Set("X-Large", strings.Repeat("a", 17000))
			fmt.Fprint(w, "ok")
		case "/echo":
			data, _ := io.ReadAll(r.Body)
			fmt.Fprint(w, string(data))
		default:
			fmt.Fprint(w, "ok")
		}
	}))
	defer srv.Close()
	r := runtimeFor(t, testHandler(t, srv.URL))
	for _, tc := range []struct {
		path, id string
		wantErr  bool
		healthy  bool
	}{
		{"/ok", "http.request", false, false}, {"/exact", "http.request", false, false},
		{"/error", "http.request", true, false}, {"/redirect", "http.request", true, false},
		{"/large", "http.request", true, false}, {"/binary", "http.request", true, false},
		{"/compressed", "http.request", true, false}, {"/huge-headers", "http.request", true, false},
		{"/ok", "http.health_check", false, true}, {"/error", "http.health_check", false, false},
		{"/redirect", "http.health_check", false, false}, {"/large", "http.health_check", true, false},
	} {
		t.Run(tc.id+tc.path, func(t *testing.T) {
			before := hits.Load()
			out, err := r.Invoke(t.Context(), tc.id, "1", map[string]any{"url": srv.URL + tc.path})
			if (err != nil) != tc.wantErr || strings.Contains(string(out), "secret-marker") {
				t.Fatalf("%s %v", out, err)
			}
			if hits.Load() != before+1 {
				t.Fatal("unexpected follow-up request")
			}
			if err == nil && tc.id == "http.health_check" {
				var got map[string]any
				if err := json.Unmarshal(out, &got); err != nil {
					t.Fatal(err)
				}
				if got["healthy"] != tc.healthy {
					t.Fatal(got)
				}
			}
		})
	}
	out, err := r.Invoke(t.Context(), "http.request", "1", map[string]any{"url": srv.URL + "/echo", "method": "POST", "body": strings.Repeat("x", 8192)})
	if err != nil || !strings.Contains(string(out), strings.Repeat("x", 8192)) {
		t.Fatal(err)
	}
	before := hits.Load()
	for _, input := range []map[string]any{
		{"url": srv.URL, "method": "TRACE"}, {"url": srv.URL, "method": "get"},
		{"url": srv.URL, "body": "no"}, {"url": srv.URL, "method": "HEAD", "body": "no"},
		{"url": srv.URL, "method": "POST", "body": strings.Repeat("x", 8193)},
		{"url": srv.URL, "unknown": "secret-marker"}, {"url": "http://example.com"},
		{"url": srv.URL, "headers": []any{map[string]any{"name": "Authorization", "value": "secret-marker"}}},
	} {
		if out, err := r.Invoke(t.Context(), "http.request", "1", input); err == nil || strings.Contains(string(out), "secret-marker") {
			t.Fatal(string(out), err)
		}
	}
	if hits.Load() != before {
		t.Fatal("invalid inputs made external requests")
	}
}
func TestHeaderRules(t *testing.T) {
	for _, name := range []string{"authorization", "COOKIE", "Proxy-Authorization", "Host", "Content-Length", "Transfer-Encoding", "Connection", "Upgrade", "Trailer", "Accept-Encoding", "X-Api-Key", "X-Auth-Token", "bad name", ""} {
		if _, err := headers([]any{map[string]any{"name": name, "value": "secret-marker"}}); err == nil {
			t.Errorf("allowed %s", name)
		}
	}
	for _, v := range []string{"x\r\nInjected: true", "\x00", "\x7f", "é"} {
		if _, err := headers([]any{map[string]any{"name": "X-Test", "value": v}}); err == nil {
			t.Fatal("invalid header value accepted")
		}
	}
	entry := func(name, v string) any { return map[string]any{"name": name, "value": v} }
	for _, entries := range [][]any{{entry("X-Test", "a"), entry("x-test", "b")}, {entry("X", strings.Repeat("a", 4096))}, make([]any, 17)} {
		if _, err := headers(entries); err == nil {
			t.Fatal("header limit accepted")
		}
	}
	if _, err := headers([]any{entry("X", strings.Repeat("a", 4095))}); err != nil {
		t.Fatal(err)
	}
	good := make([]any, 16)
	for i := range good {
		good[i] = entry(fmt.Sprintf("X-%d", i), "ok")
	}
	if _, err := headers(good); err != nil {
		t.Fatal(err)
	}
}
func TestHTTPContextTLSAndProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	entered := make(chan struct{}, 2)
	exited := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done()
		exited <- struct{}{}
	}))
	defer srv.Close()
	r := runtimeFor(t, testHandler(t, srv.URL))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := r.Invoke(ctx, "http.request", "1", map[string]any{"url": srv.URL}); done <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("request did not arrive; proxy used?")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation hung")
	}
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatal("request not released")
	}
	deadline, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	if _, err := r.Invoke(deadline, "http.health_check", "1", map[string]any{"url": srv.URL}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS reached handler") }))
	defer tls.Close()
	if _, err := runtimeFor(t, testHandler(t, tls.URL)).Invoke(t.Context(), "http.request", "1", map[string]any{"url": tls.URL}); err == nil {
		t.Fatal("invalid TLS trusted")
	}
}
func FuzzHTTPURLPolicy(f *testing.F) {
	for _, seed := range []string{"https://example.com/path", "http://127.0.0.1", "http://[::1]", "http://user:pass@host", "http://[::ffff:127.0.0.1]"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 16384 {
			return
		}
		u, origin, err := parseURL(raw)
		if err == nil {
			if u == nil || origin == "" {
				t.Fatal("invalid accepted URL")
			}
			if _, again, e := parseURL(u.String()); e != nil || again != origin {
				t.Fatal("unstable normalization")
			}
		}
		_ = (Config{AllowedOrigins: []string{raw}, AllowedPrivateCIDRs: []string{raw}}).Validate()
	})
}
func BenchmarkHTTPPolicy(b *testing.B) {
	p, err := newPolicy(Config{Enabled: true, AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, origin, err := parseURL("https://example.com/health")
		if err != nil || !p.origins[origin] || !p.allowsIP(netip.MustParseAddr("8.8.8.8")) {
			b.Fatal("policy")
		}
	}
}

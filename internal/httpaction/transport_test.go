package httpaction

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

func TestPinnedHostnameAndHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "service.example:80" || r.Header.Get("X-Test") != "safe" || r.Header.Get("Accept-Encoding") != "" {
			t.Errorf("unexpected request host/header")
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	h := testHandler(t, "http://service.example")
	h.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	h.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "8.8.8.8:80" {
			t.Errorf("unchecked dial: %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	r := runtimeFor(t, h)
	if _, err := r.Invoke(t.Context(), "http.request", "1", map[string]any{"url": "http://service.example/", "headers": []any{map[string]any{"name": "X-Test", "value": "safe"}}}); err != nil {
		t.Fatal(err)
	}
}
func TestResponseAndConnectionDeadlines(t *testing.T) {
	for _, body := range []bool{false, true} {
		t.Run(fmt.Sprint(body), func(t *testing.T) {
			entered := make(chan struct{})
			released := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if body {
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				close(entered)
				<-r.Context().Done()
				close(released)
			}))
			defer srv.Close()
			r := runtimeFor(t, testHandler(t, srv.URL))
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			_, err := r.Invoke(ctx, "http.request", "1", map[string]any{"url": srv.URL})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			select {
			case <-entered:
			default:
				t.Fatal("deadline before request")
			}
			select {
			case <-released:
			case <-time.After(3 * time.Second):
				t.Fatal("deadline left request active")
			}
		})
	}
	h := testHandler(t, "http://service.example")
	h.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	h.dial = func(ctx context.Context, _, _ string) (net.Conn, error) { <-ctx.Done(); return nil, ctx.Err() }
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := h.connect(ctx, "tcp", "service.example:80"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestEscapedOutputAndTruncatedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/truncated" {
			w.Header().Set("Content-Length", "20")
			fmt.Fprint(w, "short")
			return
		}
		fmt.Fprint(w, strings.Repeat("\x00", 8192))
	}))
	defer srv.Close()
	r := runtimeFor(t, testHandler(t, srv.URL))
	out, err := r.Invoke(t.Context(), "http.request", "1", map[string]any{"url": srv.URL})
	if err != nil || len(out) > 65536 || !strings.Contains(string(out), `\u0000`) {
		t.Fatal(len(out), err)
	}
	if _, err := r.Invoke(t.Context(), "http.request", "1", map[string]any{"url": srv.URL + "/truncated"}); err != execution.ErrAction {
		t.Fatal(err)
	}
}

package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/event"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

func signedRequest(key, path, body string, when time.Time) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	stamp := strconv.FormatInt(when.Unix(), 10)
	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write([]byte(stamp + "\nunique-id\n" + path + "\n" + body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-OpsWeave-Timestamp", stamp)
	r.Header.Set("X-OpsWeave-Event-ID", "unique-id")
	r.Header.Set("X-OpsWeave-Signature", hex.EncodeToString(h.Sum(nil)))
	return r
}

func TestWebhookAuthenticationAndBounds(t *testing.T) {
	const key = "01234567890123456789012345678901"
	const path = "/v1/hooks/example/incoming"
	now := time.Unix(1800000000, 0)
	for _, tc := range []struct {
		name, path, body string
		offset           time.Duration
		change           func(*http.Request)
		status           int
	}{
		{"valid", path, `{"secret":"payload"}`, 0, nil, 202},
		{"tampered", path, `{}`, 0, func(r *http.Request) { r.Header.Set("X-OpsWeave-Event-ID", "wrong") }, 401},
		{"expired", path, `{}`, -301 * time.Second, nil, 401},
		{"future", path, `{}`, 301 * time.Second, nil, 401},
		{"duplicate", path, `{}`, 0, func(r *http.Request) { r.Header.Add("X-OpsWeave-Timestamp", "1800000000") }, 401},
		{"array", path, `[]`, 0, nil, 400},
		{"trailing", path, `{} {}`, 0, nil, 400},
		{"too large", path, `{"x":"` + strings.Repeat("a", 64<<10) + `"}`, 0, nil, 413},
		{"headers too large", path, `{}`, 0, func(r *http.Request) { r.Header.Set("X-Large", strings.Repeat("a", HeaderLimit)) }, 431},
		{"content encoding", path, `{}`, 0, func(r *http.Request) { r.Header.Set("Content-Encoding", "identity") }, 415},
		{"wrong media", path, `{}`, 0, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		{"query", path + "?x=1", `{}`, 0, nil, 404},
		{"encoded path", "/v1/hooks/exam%70le/incoming", `{}`, 0, nil, 404},
		{"method", path, `{}`, 0, func(r *http.Request) { r.Method = http.MethodGet }, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h, err := New([]byte(key), func(_ context.Context, in event.Request) ([]event.Receipt, error) {
				calls++
				if in.Workflow != "example" || in.Trigger != "incoming" || in.Fingerprint != event.Digest([]byte(tc.body)) {
					t.Fatal(in)
				}
				return []event.Receipt{{Execution: "execution-id"}}, nil
			}, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			r := signedRequest(key, tc.path, tc.body, now.Add(tc.offset))
			if tc.change != nil {
				tc.change(r)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || (calls == 1) != (tc.status == 202) || strings.Contains(w.Body.String(), "payload") {
				t.Fatal(w.Code, calls, w.Body.String())
			}
		})
	}
}

func TestWebhookStatusMappingAndConcurrentLimit(t *testing.T) {
	const key = "01234567890123456789012345678901"
	const path = "/v1/hooks/example/incoming"
	now := time.Unix(1800000000, 0)
	for _, tc := range []struct {
		err    error
		status int
	}{{event.ErrRoute, 404}, {event.ErrConflict, 409}, {execution.ErrQueueFull, 503}, {context.DeadlineExceeded, 503}} {
		h, err := New([]byte(key), func(context.Context, event.Request) ([]event.Receipt, error) { return nil, tc.err }, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedRequest(key, path, "{}", now))
		if w.Code != tc.status || (w.Code == 503 && w.Header().Get("Retry-After") == "") {
			t.Fatal(w.Code, w.Header())
		}
	}
	started := make(chan struct{}, 16)
	release := make(chan struct{})
	h, err := New([]byte(key), func(context.Context, event.Request) ([]event.Receipt, error) {
		started <- struct{}{}
		<-release
		return nil, nil
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() { h.ServeHTTP(httptest.NewRecorder(), signedRequest(key, path, "{}", now)) })
	}
	defer func() { close(release); wg.Wait() }()
	for range 16 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("handler never started")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(key, path, "{}", now))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func FuzzWebhookRoute(f *testing.F) {
	f.Add("/v1/hooks/example/incoming")
	f.Add("/v1/hooks/../%2f")
	f.Fuzz(func(t *testing.T, path string) {
		workflow, trigger, ok := route(path)
		if ok && (!event.ValidName(workflow) || !event.ValidName(trigger) || path != "/v1/hooks/"+workflow+"/"+trigger) {
			t.Fatal("noncanonical route")
		}
	})
}

func BenchmarkWebhookAuthentication(b *testing.B) {
	const key = "01234567890123456789012345678901"
	now := time.Unix(1800000000, 0)
	h, err := New([]byte(key), func(context.Context, event.Request) ([]event.Receipt, error) {
		return []event.Receipt{{Execution: "example-id"}}, nil
	}, func() time.Time { return now })
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedRequest(key, "/v1/hooks/example/incoming", "{}", now))
		if w.Code != 202 {
			b.Fatal(w.Code)
		}
	}
}

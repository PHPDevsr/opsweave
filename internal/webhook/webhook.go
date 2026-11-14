// Package webhook implements the authenticated loopback ingress protocol.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/event"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

const bodyLimit = 64 << 10

// HeaderLimit bounds the request line and decoded headers accepted for ingress.
const HeaderLimit = 8 << 10

// New authenticates the exact signed request before calling the admission function.
func New(secret []byte, admit func(context.Context, event.Request) ([]event.Receipt, error), now func() time.Time) (http.Handler, error) {
	if len(secret) < 32 || len(secret) > 1024 || admit == nil {
		return nil, errors.New("webhook requires a signing secret of 32 to 1024 bytes and an admission handler")
	}
	key := bytes.Clone(secret)
	if now == nil {
		now = time.Now
	}
	active := make(chan struct{}, 16)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case active <- struct{}{}:
			defer func() { <-active }()
		default:
			reject(w, http.StatusServiceUnavailable, "busy")
			return
		}
		serve(w, r, key, admit, now())
	}), nil
}

func serve(w http.ResponseWriter, r *http.Request, key []byte, admit func(context.Context, event.Request) ([]event.Receipt, error), now time.Time) {
	size := len(r.Method) + len(r.RequestURI) + len(r.Proto) + len(r.Host) + 12
	for name, values := range r.Header {
		for _, value := range values {
			size += len(name) + len(value) + 4
		}
	}
	if size > HeaderLimit {
		reject(w, http.StatusRequestHeaderFieldsTooLarge, "headers_too_large")
		return
	}
	workflow, trigger, ok := route(r.RequestURI)
	if !ok {
		reject(w, http.StatusNotFound, "route_unavailable")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		reject(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	media, _, err := mime.ParseMediaType(single(r.Header, "Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Encoding")) != 0 {
		reject(w, http.StatusUnsupportedMediaType, "unsupported_media")
		return
	}
	stamp := single(r.Header, "X-OpsWeave-Timestamp")
	id := single(r.Header, "X-OpsWeave-Event-ID")
	signature := single(r.Header, "X-OpsWeave-Signature")
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || seconds < 0 || strconv.FormatInt(seconds, 10) != stamp || !event.ValidID(id) || len(signature) != 64 || outsideWindow(seconds, now.Unix()) {
		reject(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		reject(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyLimit))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			reject(w, http.StatusRequestEntityTooLarge, "body_too_large")
		} else {
			reject(w, http.StatusBadRequest, "invalid_body")
		}
		return
	}
	mac := hmac.New(sha256.New, key)
	// hash.Hash.Write accepts every byte and always returns a nil error.
	mac.Write([]byte(stamp + "\n" + id + "\n" + r.RequestURI + "\n"))
	mac.Write(body)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		reject(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' || !utf8.Valid(body) || !json.Valid(body) {
		reject(w, http.StatusBadRequest, "invalid_body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	receipts, err := admit(ctx, event.Request{Type: "webhook", ID: id, Workflow: workflow, Trigger: trigger, Fingerprint: event.Digest(body)})
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		// A disconnected client can retry the durable admission with its original event ID.
		if err := json.NewEncoder(w).Encode(receipts); err != nil {
			return
		}
	case errors.Is(err, event.ErrRoute):
		reject(w, http.StatusNotFound, "route_unavailable")
	case errors.Is(err, event.ErrConflict):
		reject(w, http.StatusConflict, "event_conflict")
	case errors.Is(err, execution.ErrQueueFull):
		reject(w, http.StatusServiceUnavailable, "queue_full")
	default:
		reject(w, http.StatusServiceUnavailable, "admission_unavailable")
	}
}

func route(uri string) (string, string, bool) {
	rest, ok := strings.CutPrefix(uri, "/v1/hooks/")
	if !ok {
		return "", "", false
	}
	workflow, trigger, ok := strings.Cut(rest, "/")
	return workflow, trigger, ok && event.ValidName(workflow) && event.ValidName(trigger)
}

func single(headers http.Header, key string) string {
	values := headers.Values(key)
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func outsideWindow(seconds, now int64) bool {
	return seconds < now-300 || seconds > now+300
}

func reject(w http.ResponseWriter, status int, code string) {
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "1")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, `{"error":"`+code+`"}`+"\n"); err != nil {
		return
	}
}

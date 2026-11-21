package httpaction

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/registry"
)

const bodyLimit = 8192

// Runtime composes exact-version registrations and explicit grants. Disabled
// configuration yields an empty runtime/catalog without network access.
func Runtime(c Config) (*action.Runtime, registry.Catalog, error) {
	p, err := newPolicy(c)
	if err != nil {
		return nil, nil, err
	}
	if !c.Enabled {
		r, e := action.New(nil, nil)
		return r, registry.Catalog{}, e
	}
	h := handler{policy: p, lookup: net.DefaultResolver.LookupNetIP, dial: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}
	entries := h.registrations()
	r, err := action.New(entries, []string{"http.request", "http.health_check"})
	if err != nil {
		return nil, nil, err
	}
	catalog, err := r.Catalog(map[string]string{"http.request": "1", "http.health_check": "1"})
	return r, catalog, err
}

type handler struct {
	policy policy
	lookup func(context.Context, string, string) ([]netip.Addr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func (h handler) registrations() []action.Registration {
	var entries []action.Registration
	for _, id := range []string{"http.request", "http.health_check"} {
		inputs := map[string]registry.Field{"url": {Type: "string", Required: true}}
		outputs := map[string]registry.Field{"status_code": {Type: "integer", Required: true}}
		if id == "http.request" {
			inputs["method"] = registry.Field{Type: "string"}
			inputs["body"] = registry.Field{Type: "string"}
			inputs["headers"] = registry.Field{Type: "array", Items: &registry.Field{Type: "object", Properties: map[string]registry.Field{"name": {Type: "string", Required: true}, "value": {Type: "string", Required: true}}}}
			outputs["body"] = registry.Field{Type: "string", Required: true}
		} else {
			outputs["healthy"] = registry.Field{Type: "boolean", Required: true}
		}
		entries = append(entries, action.Registration{Descriptor: registry.Descriptor{ID: id, Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: inputs}, Outputs: outputs, Permissions: []string{id}}, Run: func(ctx context.Context, in map[string]any) (map[string]any, error) {
			return h.run(ctx, in, id == "http.health_check")
		}})
	}
	return entries
}

func token(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}

func headers(value any) (http.Header, error) {
	out := http.Header{}
	if value == nil {
		return out, nil
	}
	entries, ok := value.([]any)
	if !ok || len(entries) > 16 {
		return nil, errPolicy
	}
	total := 0
	for _, entry := range entries {
		fields, ok := entry.(map[string]any)
		if !ok || len(fields) != 2 {
			return nil, errPolicy
		}
		name, ok := fields["name"].(string)
		if !ok || !token(name) {
			return nil, errPolicy
		}
		name = http.CanonicalHeaderKey(name)
		v, ok := fields["value"].(string)
		if !ok || !utf8.ValidString(v) {
			return nil, errPolicy
		}
		for _, ch := range v {
			if ch < 32 || ch == 127 || ch > 126 {
				return nil, errPolicy
			}
		}
		switch name {
		case "Authorization", "Proxy-Authorization", "X-Api-Key", "Api-Key", "X-Auth-Token", "Cookie", "Set-Cookie", "Host", "Content-Length", "Connection", "Transfer-Encoding", "Upgrade", "Trailer", "Accept-Encoding", "Te", "Proxy-Connection":
			return nil, errPolicy
		}
		if _, exists := out[name]; exists {
			return nil, errPolicy
		}
		total += len(name) + len(v)
		if total > 4096 {
			return nil, errPolicy
		}
		out[name] = []string{v}
	}
	return out, nil
}

func (h handler) connect(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errPolicy
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var addresses []netip.Addr
	if ip, e := netip.ParseAddr(host); e == nil {
		addresses = []netip.Addr{ip}
	} else {
		addresses, err = h.lookup(ctx, "ip", host)
		if err != nil {
			return nil, errPolicy
		}
	}
	if len(addresses) == 0 || len(addresses) > 64 {
		return nil, errPolicy
	}
	for _, ip := range addresses {
		if !h.policy.allowsIP(ip) {
			return nil, errPolicy
		}
	}
	for _, ip := range addresses {
		conn, e := h.dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("HTTP action connection failed")
}

func (h handler) run(ctx context.Context, in map[string]any, health bool) (map[string]any, error) {
	raw, ok := in["url"].(string)
	if !ok {
		return nil, errPolicy
	}
	u, origin, err := parseURL(raw)
	if err != nil || !h.policy.origins[origin] {
		return nil, errPolicy
	}
	method := "GET"
	if v, exists := in["method"]; exists {
		var valid bool
		method, valid = v.(string)
		if !valid {
			return nil, errPolicy
		}
	}
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
	default:
		return nil, errPolicy
	}
	body := ""
	if v, exists := in["body"]; exists {
		var valid bool
		body, valid = v.(string)
		if !valid {
			return nil, errPolicy
		}
	}
	if len(body) > bodyLimit || !utf8.ValidString(body) || ((method == "GET" || method == "HEAD") && body != "") {
		return nil, errPolicy
	}
	head, err := headers(in["headers"])
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), strings.NewReader(body))
	if err != nil {
		return nil, errPolicy
	}
	req.Header = head
	transport := &http.Transport{Proxy: nil, DialContext: h.connect, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 16 << 10, DisableCompression: true, DisableKeepAlives: true, MaxConnsPerHost: 1}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("HTTP action request failed")
	}

	data, readErr := io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(data) > bodyLimit || !utf8.Valid(data) || response.Header.Get("Content-Encoding") != "" {
		return nil, errors.New("HTTP action response rejected")
	}
	result := map[string]any{"status_code": int64(response.StatusCode)}
	if health {
		result["healthy"] = response.StatusCode >= 200 && response.StatusCode <= 299
	} else {
		if response.StatusCode < 200 || response.StatusCode > 299 {
			return nil, errors.New("HTTP action response status failed")
		}
		result["body"] = string(data)
	}
	return result, nil
}

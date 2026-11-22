package notificationaction

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/httpaction"
	"github.com/PHPDevsr/opsweave/internal/registry"
)

// Dependencies inject trusted application infrastructure, never workflow/config
// options. Production leaves Dial and RootCAs nil and uses system network/trust.
type Dependencies struct {
	LookupEnv func(string) (string, bool)
	Dial      func(context.Context, string, string) (net.Conn, error)
	RootCAs   *x509.CertPool
}

type resolved struct {
	profile                    Profile
	destination                *httpaction.Destination
	url                        *url.URL
	username, password, bearer string
}

func validToken(s string) bool {
	id, token, ok := strings.Cut(s, ":")
	value, err := strconv.ParseUint(id, 10, 64)
	if !ok || err != nil || value == 0 || token == "" {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	for _, c := range token {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validBearer(s string) bool {
	core := strings.TrimRight(s, "=")
	if core == "" {
		return false
	}
	for _, c := range core {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~+/", c)) {
			return false
		}
	}
	return true
}

func resolve(p Profile, lookup func(string) (string, bool)) (resolved, error) {
	r := resolved{profile: p}
	r.profile.Recipients = append([]string(nil), p.Recipients...)
	r.profile.AllowedPrivateCIDRs = append([]string(nil), p.AllowedPrivateCIDRs...)
	dest, err := httpaction.NewDestination(p.origin(), p.AllowedPrivateCIDRs)
	if err != nil {
		return r, errConfig
	}
	r.destination = dest
	secret := func(name string, limit int) (string, error) {
		value, ok := lookup(name)
		if !ok || !printable(value, limit) {
			return "", errConfig
		}
		return value, nil
	}
	raw := p.origin()
	switch p.Action {
	case "notification.telegram":
		token, err := secret(p.TokenEnv, 4096)
		if err != nil || !validToken(token) {
			return r, errConfig
		}
		raw += "/bot" + token + "/sendMessage"
	case "notification.slack", "notification.webhook":
		raw, err = secret(p.URLEnv, 2048)
		if err != nil {
			return r, errConfig
		}
		if p.BearerEnv != "" {
			r.bearer, err = secret(p.BearerEnv, 4096)
			if err != nil || !validBearer(r.bearer) {
				return r, errConfig
			}
		}
	case "notification.email":
		r.username, err = secret(p.UsernameEnv, 4096)
		if err != nil {
			return r, errConfig
		}
		r.password, err = secret(p.PasswordEnv, 4096)
		if err != nil {
			return r, errConfig
		}
	}
	if len(raw) > 2048 {
		return r, errConfig
	}
	r.url, err = dest.URL(raw)
	if err != nil {
		return r, errConfig
	}
	if p.Action == "notification.slack" {
		if r.url.RawPath != "" || r.url.RawQuery != "" || r.url.ForceQuery || !strings.HasPrefix(r.url.Path, "/services/") {
			return r, errConfig
		}
		parts := strings.Split(strings.TrimPrefix(r.url.Path, "/services/"), "/")
		if len(parts) != 3 {
			return r, errConfig
		}
		for _, part := range parts {
			if part == "" {
				return r, errConfig
			}
			for _, c := range part {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
					return r, errConfig
				}
			}
		}
	}
	return r, nil
}

// Registrations snapshots private credentials only for enabled, validated profiles.
// No resolved credential is included in exported config, descriptors or outputs.
func Registrations(c Config, deps Dependencies) ([]action.Registration, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, nil
	}
	if deps.LookupEnv == nil {
		deps.LookupEnv = os.LookupEnv
	}
	if deps.RootCAs != nil {
		deps.RootCAs = deps.RootCAs.Clone()
	}
	profiles := map[string]resolved{}
	actions := map[string]bool{}
	for _, p := range c.Profiles {
		r, err := resolve(p, deps.LookupEnv)
		if err != nil {
			return nil, errConfig
		}
		profiles[p.Name] = r
		actions[p.Action] = true
	}
	var entries []action.Registration
	for _, id := range []string{"notification.telegram", "notification.slack", "notification.email", "notification.webhook"} {
		if !actions[id] {
			continue
		}
		inputs := map[string]registry.Field{"profile": {Type: "string", Required: true}, "message": {Type: "string", Required: true}}
		if id == "notification.email" {
			inputs["subject"] = registry.Field{Type: "string", Required: true}
		}
		entries = append(entries, action.Registration{Descriptor: registry.Descriptor{ID: id, Version: "1", APIVersion: "opsweave.io/v1", Permissions: []string{id}, Inputs: registry.Field{Type: "object", Properties: inputs}, Outputs: map[string]registry.Field{"accepted": {Type: "boolean", Required: true}}}, Run: func(ctx context.Context, in map[string]any) (map[string]any, error) {
			name, ok := in["profile"].(string)
			if !ok {
				return nil, errAction
			}
			p, exists := profiles[name]
			if !exists || p.profile.Action != id {
				return nil, errAction
			}
			return deliver(ctx, p, deps, in)
		}})
	}
	return entries, nil
}

type ownedConn struct {
	net.Conn
	once sync.Once
	err  error
}

func (c *ownedConn) Close() error { c.once.Do(func() { c.err = c.Conn.Close() }); return c.err }

type receiveConn struct {
	net.Conn
	remaining int64
}

func (c *receiveConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if c.remaining <= 0 {
		return 0, errAction
	}
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.Conn.Read(p)
	c.remaining -= int64(n)
	return n, err
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func deliver(parent context.Context, p resolved, deps Dependencies, in map[string]any) (output map[string]any, resultErr error) {
	message, ok := in["message"].(string)
	want := 2
	if p.profile.Action == "notification.email" {
		want = 3
	}
	if !ok || len(message) == 0 || len(message) > 4096 || !utf8.ValidString(message) || len(in) != want {
		return nil, errAction
	}
	subject := ""
	if want == 3 {
		subject, ok = in["subject"].(string)
		if !ok || !printable(subject, 256) {
			return nil, errAction
		}
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	defer func() {
		// Native I/O deadlines may wake before the context timer propagates.
		if !time.Now().Before(deadline) {
			<-ctx.Done()
		}
		if resultErr != nil || ctx.Err() != nil {
			output = nil
			resultErr = errAction
		}
	}()
	if ctx.Err() != nil {
		return nil, errAction
	}
	dialCtx, stopDial := context.WithTimeout(ctx, 5*time.Second)
	dial := p.destination.DialContext
	if deps.Dial != nil {
		dial = deps.Dial
	}
	conn, err := dial(dialCtx, "tcp", p.url.Host)
	stopDial()
	if err != nil {
		if conn != nil {
			err = errors.Join(err, conn.Close())
		}
		return nil, err
	}
	if conn == nil {
		return nil, errAction
	}
	owned := &ownedConn{Conn: conn}
	closed := make(chan struct{})
	// The callback stores its close error in owned; joined cleanup reads it.
	stopClose := context.AfterFunc(ctx, func() { defer close(closed); _ = owned.Close() })
	defer func() {
		if !stopClose() {
			<-closed
		}
		resultErr = errors.Join(resultErr, owned.Close())
	}()
	if owned.SetDeadline(earlier(deadline, time.Now().Add(5*time.Second))) != nil {
		return nil, errAction
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: p.url.Hostname(), RootCAs: deps.RootCAs}
	if p.profile.Action == "notification.email" {
		err = sendEmail(ctx, &receiveConn{Conn: owned, remaining: 256 << 10}, p, config, subject, message, deadline)
	} else {
		tlsConn := tls.Client(owned, config)
		if err = tlsConn.HandshakeContext(ctx); err == nil {
			err = sendHTTP(ctx, tlsConn, owned, p, message, deadline)
		}
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"accepted": true}, nil
}

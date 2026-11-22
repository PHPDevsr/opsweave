package notificationaction

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

func TestProfileValidation(t *testing.T) {
	base := webhookProfile("https://example.com")
	for _, tc := range []struct {
		name   string
		change func(*Profile)
	}{
		{"name", func(p *Profile) { p.Name = "../ops" }}, {"action", func(p *Profile) { p.Action = "notification.other" }},
		{"reference-prefix", func(p *Profile) { p.URLEnv = "TOKEN" }}, {"reference-case", func(p *Profile) { p.URLEnv = testURL + "lower" }},
		{"reference-length", func(p *Profile) { p.URLEnv = strings.Repeat("A", 129) }}, {"reference-empty", func(p *Profile) { p.URLEnv = secretPrefix }},
		{"cross-channel", func(p *Profile) { p.ChatID = "1" }}, {"cross-channel-smtp", func(p *Profile) { p.From = "from@example.com" }},
		{"http", func(p *Profile) { p.AllowedOrigin = "http://example.com" }}, {"origin-path", func(p *Profile) { p.AllowedOrigin += "/secret" }},
		{"cidr", func(p *Profile) { p.AllowedPrivateCIDRs = []string{"0.0.0.0/0"} }},
		{"bearer-reference", func(p *Profile) { p.BearerEnv = "secret" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.change(&p)
			if (Config{Enabled: true, Profiles: []Profile{p}}).Validate() == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
	for _, c := range []Config{{Enabled: true}, {Profiles: []Profile{base, base}}, {Profiles: make([]Profile, 129)}} {
		if c.Validate() == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	if (Config{}).Validate() != nil {
		t.Fatal("default invalid")
	}
	for _, chat := range []string{"0", "+1", "01", "1.0", "9223372036854775808"} {
		p := Profile{Name: "ops", Action: "notification.telegram", TokenEnv: testToken, ChatID: chat}
		if p.validate() == nil {
			t.Fatal("invalid chat accepted")
		}
	}
	p := Profile{Name: "ops", Action: "notification.email", Host: "localhost", Port: 465, TLSMode: "implicit_tls", UsernameEnv: testURL, PasswordEnv: testToken, From: "from@example.com", Recipients: []string{"to@example.com"}}
	if p.validate() != nil {
		t.Fatal("valid email rejected")
	}
	for _, change := range []func(*Profile){func(p *Profile) { p.Port = 0 }, func(p *Profile) { p.TLSMode = "plain" }, func(p *Profile) { p.From = "Name <from@example.com>" }, func(p *Profile) { p.Recipients = []string{"to@example.com\r\nBcc: bad@example.com"} }, func(p *Profile) { p.Recipients = []string{"to@example.com", "to@example.com"} }, func(p *Profile) { p.Recipients = nil }, func(p *Profile) { p.UsernameEnv = "USER" }} {
		q := p
		change(&q)
		if q.validate() == nil {
			t.Fatal("unsafe email accepted")
		}
	}
}

func TestCredentialResolutionAndRedaction(t *testing.T) {
	for _, tc := range []struct{ action, secret string }{
		{"notification.telegram", "0:secret"}, {"notification.telegram", "12:bad/secret"}, {"notification.telegram", strings.Repeat("1", 4100)},
		{"notification.slack", "https://hooks.slack.com/services/T/B/secret?injected=yes"}, {"notification.slack", "https://evil.example/services/T/B/secret"}, {"notification.slack", "https://hooks.slack.com/services/T/B/%73ecret"}, {"notification.slack", "https://hooks.slack.com/services/T/B/secret/extra"},
		{"notification.webhook", "http://example.com/secret"}, {"notification.webhook", "https://user:secret@example.com/"}, {"notification.webhook", "https://example.com/#secret"}, {"notification.webhook", "https://example.com/\r\nsecret"}, {"notification.webhook", strings.Repeat("s", 2049)},
	} {
		t.Run(tc.action+tc.secret[:min(len(tc.secret), 20)], func(t *testing.T) {
			p := webhookProfile("https://example.com")
			p.Action = tc.action
			if tc.action != "notification.webhook" {
				p.AllowedOrigin = ""
				p.AllowedPrivateCIDRs = nil
			}
			if tc.action == "notification.telegram" {
				p.URLEnv = ""
				p.TokenEnv = testToken
				p.ChatID = "1"
			}
			_, err := Registrations(Config{Enabled: true, Profiles: []Profile{p}}, Dependencies{LookupEnv: func(string) (string, bool) { return tc.secret, true }})
			if err == nil || strings.Contains(err.Error(), tc.secret) {
				t.Fatal("missing or unsafe credential error")
			}
		})
	}
	p := webhookProfile("https://example.com")
	for _, value := range []string{"", "secret\x00value", string([]byte{0xff})} {
		_, err := Registrations(Config{Enabled: true, Profiles: []Profile{p}}, Dependencies{LookupEnv: func(string) (string, bool) { return value, true }})
		if !errors.Is(err, errConfig) {
			t.Fatal(err)
		}
	}
	_, err := Registrations(Config{Enabled: true, Profiles: []Profile{p}}, Dependencies{LookupEnv: func(string) (string, bool) { return "", false }})
	if err == nil {
		t.Fatal("missing secret accepted")
	}
	entries, err := Registrations(Config{}, Dependencies{LookupEnv: func(string) (string, bool) { t.Fatal("disabled reads credentials"); return "", false }})
	if err != nil || len(entries) != 0 {
		t.Fatal("default enabled")
	}
	for _, bearer := range []string{"bad space", "=", "bad\r\nheader", "bad:token"} {
		p.BearerEnv = testToken
		_, err := Registrations(Config{Enabled: true, Profiles: []Profile{p}}, Dependencies{LookupEnv: func(k string) (string, bool) {
			if k == testToken {
				return bearer, true
			}
			return "https://example.com", true
		}})
		if err == nil {
			t.Fatal("invalid bearer accepted")
		}
	}
	data, err := json.Marshal(Config{Enabled: true, Profiles: []Profile{p}})
	if err != nil || strings.Contains(string(data), "synthetic-secret-value") {
		t.Fatal("configuration leak")
	}
}

func TestInputsPermissionsAndVersionsBeforeDial(t *testing.T) {
	p := webhookProfile("https://example.com")
	deps := Dependencies{LookupEnv: func(string) (string, bool) { return "https://example.com/secret", true }, Dial: func(context.Context, string, string) (net.Conn, error) {
		t.Error("invalid invocation dialed")
		return nil, errors.New("unexpected dial")
	}}
	entries, err := Registrations(Config{Enabled: true, Profiles: []Profile{p}}, deps)
	if err != nil {
		t.Fatal(err)
	}
	runtime := runtimeFor(t, p, deps)
	for _, in := range []map[string]any{{"profile": "ops", "message": "", "headers": "override"}, {"profile": "ops", "message": ""}, {"profile": "ops", "message": strings.Repeat("x", 4097)}, {"profile": "missing", "message": "hi"}, {"profile": "ops", "message": string([]byte{0xff})}, {"profile": "ops", "message": 1}, {"message": "hi"}} {
		out, err := runtime.Invoke(t.Context(), p.Action, "1", in)
		if err == nil || len(out) != 0 {
			t.Fatal("invalid input accepted")
		}
	}
	denied, err := action.New(entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := denied.Invoke(t.Context(), p.Action, "1", map[string]any{"profile": "ops", "message": "hi"}); !errors.Is(err, execution.ErrPermission) {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(t.Context(), p.Action, "2", nil); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := runtime.Invoke(ctx, p.Action, "1", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Profile/channel mismatch is checked by the handler even with otherwise valid input.
	telegram := Profile{Name: "telegram", Action: "notification.telegram", TokenEnv: testToken, ChatID: "1"}
	deps.LookupEnv = func(k string) (string, bool) {
		if k == testToken {
			return "12:synthetic", true
		}
		return "https://example.com", true
	}
	both, err := Registrations(Config{Enabled: true, Profiles: []Profile{p, telegram}}, deps)
	if err != nil {
		t.Fatal(err)
	}
	r, err := action.New(both, []string{p.Action, telegram.Action})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Invoke(t.Context(), p.Action, "1", map[string]any{"profile": "telegram", "message": "hi"}); err == nil {
		t.Fatal("cross-channel profile allowed")
	}
	email := Profile{Name: "mail", Action: "notification.email", Host: "localhost", Port: 465, TLSMode: "implicit_tls", UsernameEnv: testURL, PasswordEnv: testToken, From: "from@example.com", Recipients: []string{"to@example.com"}}
	deps.LookupEnv = func(string) (string, bool) { return "synthetic", true }
	mailRuntime := runtimeFor(t, email, deps)
	for _, subject := range []string{"", "hi\r\nBcc: bad@example.com", strings.Repeat("x", 257)} {
		if _, err := mailRuntime.Invoke(t.Context(), email.Action, "1", map[string]any{"profile": "mail", "message": "hi", "subject": subject}); err == nil {
			t.Fatal("invalid subject accepted")
		}
	}
}

func FuzzNotificationCredentialParsers(f *testing.F) {
	for _, s := range []string{"123:token", "https://hooks.slack.com/services/T/B/secret", "\r\n", "ops@example.com"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8192 {
			return
		}
		_ = validToken(s)
		_ = validBearer(s)
		_ = mailbox(s)
		_ = validReference(s)
		p := Profile{Name: "ops", Action: "notification.slack", URLEnv: testURL}
		_, _ = resolve(p, func(string) (string, bool) { return s, true })
	})
}

// Package notificationaction supplies fixed-profile, environment-credential delivery actions.
package notificationaction

import (
	"errors"
	"net"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/httpaction"
)

// Profile contains only destinations, grants and nonsecret environment references.
type Profile struct {
	Name                string
	Action              string
	ChatID              string
	TokenEnv            string
	URLEnv              string
	BearerEnv           string
	AllowedOrigin       string
	AllowedPrivateCIDRs []string
	Host                string
	Port                int
	TLSMode             string
	UsernameEnv         string
	PasswordEnv         string
	From                string
	Recipients          []string
}

// Config is disabled by default and never contains resolved credentials.
type Config struct {
	Enabled  bool
	Profiles []Profile
}

var errConfig = errors.New("invalid notification action configuration")
var errAction = errors.New("notification action failed")

const secretPrefix = "OPSWEAVE_ACTION_SECRET_"

func validName(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validAction(s string) bool {
	return s == "notification.telegram" || s == "notification.slack" || s == "notification.email" || s == "notification.webhook"
}

func validReference(s string) bool {
	if len(s) > 128 || !strings.HasPrefix(s, secretPrefix) || len(s) == len(secretPrefix) {
		return false
	}
	for _, c := range s[len(secretPrefix):] {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func printable(s string, limit int) bool {
	if len(s) == 0 || len(s) > limit || !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

func mailbox(s string) bool {
	if !printable(s, 254) || strings.ContainsAny(s, "<>") || strings.TrimSpace(s) != s {
		return false
	}
	for _, c := range s {
		if c > 126 {
			return false
		}
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s
}

func (p Profile) origin() string {
	switch p.Action {
	case "notification.telegram":
		return "https://api.telegram.org"
	case "notification.slack":
		return "https://hooks.slack.com"
	case "notification.email":
		return "https://" + net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	default:
		return p.AllowedOrigin
	}
}

func (p Profile) validate() error {
	if !validName(p.Name) || !validAction(p.Action) {
		return errConfig
	}
	// Reject fields from other channel types rather than silently ignoring them.
	if p.Action != "notification.telegram" && (p.ChatID != "" || p.TokenEnv != "") {
		return errConfig
	}
	if p.Action != "notification.slack" && p.Action != "notification.webhook" && p.URLEnv != "" {
		return errConfig
	}
	if p.Action != "notification.webhook" && (p.BearerEnv != "" || p.AllowedOrigin != "") {
		return errConfig
	}
	if p.Action != "notification.email" && (p.Host != "" || p.Port != 0 || p.TLSMode != "" || p.UsernameEnv != "" || p.PasswordEnv != "" || p.From != "" || len(p.Recipients) != 0) {
		return errConfig
	}
	if p.Action != "notification.webhook" && p.Action != "notification.email" && len(p.AllowedPrivateCIDRs) != 0 {
		return errConfig
	}
	switch p.Action {
	case "notification.telegram":
		id, err := strconv.ParseInt(p.ChatID, 10, 64)
		if err != nil || id == 0 || strconv.FormatInt(id, 10) != p.ChatID || !validReference(p.TokenEnv) {
			return errConfig
		}
	case "notification.slack", "notification.webhook":
		if !validReference(p.URLEnv) || p.BearerEnv != "" && !validReference(p.BearerEnv) {
			return errConfig
		}
	case "notification.email":
		if p.Host == "" || p.Port < 1 || p.Port > 65535 || (p.TLSMode != "implicit_tls" && p.TLSMode != "starttls") || !validReference(p.UsernameEnv) || !validReference(p.PasswordEnv) || !mailbox(p.From) || len(p.Recipients) == 0 || len(p.Recipients) > 10 {
			return errConfig
		}
		seen := map[string]bool{}
		for _, to := range p.Recipients {
			if !mailbox(to) || seen[to] {
				return errConfig
			}
			seen[to] = true
		}
	}
	if !strings.HasPrefix(p.origin(), "https://") {
		return errConfig
	}
	if _, err := httpaction.NewDestination(p.origin(), p.AllowedPrivateCIDRs); err != nil {
		return errConfig
	}
	return nil
}

// Validate checks profile policy and reference syntax without reading credentials or dialing.
func (c Config) Validate() error {
	if len(c.Profiles) > 128 || c.Enabled && len(c.Profiles) == 0 {
		return errConfig
	}
	seen := map[string]bool{}
	for _, p := range c.Profiles {
		if p.validate() != nil || seen[p.Name] {
			return errConfig
		}
		seen[p.Name] = true
	}
	return nil
}

// SecretNames returns unique nonsecret reference names for strict config environment checks.
func (c Config) SecretNames() []string {
	seen := map[string]bool{}
	for _, p := range c.Profiles {
		for _, name := range []string{p.TokenEnv, p.URLEnv, p.BearerEnv, p.UsernameEnv, p.PasswordEnv} {
			if name != "" {
				seen[name] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

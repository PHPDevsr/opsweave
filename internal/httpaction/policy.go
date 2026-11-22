// Package httpaction provides opt-in, destination-restricted HTTP actions.
package httpaction

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Config grants HTTP access only to exact origins and explicitly trusted private CIDRs.
type Config struct {
	Enabled             bool
	AllowedOrigins      []string
	AllowedPrivateCIDRs []string
}

type policy struct {
	origins map[string]bool
	private []netip.Prefix
}

var errPolicy = errors.New("HTTP action policy rejected input or destination")

// Validate checks configuration without performing network I/O.
func (c Config) Validate() error { _, err := newPolicy(c); return err }

func newPolicy(c Config) (policy, error) {
	p := policy{origins: map[string]bool{}}
	if len(c.AllowedOrigins) > 128 || len(c.AllowedPrivateCIDRs) > 128 || (c.Enabled && len(c.AllowedOrigins) == 0) {
		return p, errPolicy
	}
	for _, raw := range c.AllowedOrigins {
		u, origin, err := parseURL(raw)
		if err != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || p.origins[origin] {
			return p, errPolicy
		}
		p.origins[origin] = true
	}
	for _, raw := range c.AllowedPrivateCIDRs {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix != prefix.Masked() || prefix.Addr().Is4In6() {
			return p, errPolicy
		}
		// A grant must fit wholly inside one private/loopback range.
		valid := false
		for _, scope := range privateRanges() {
			if scope.Bits() <= prefix.Bits() && scope.Contains(prefix.Addr()) {
				valid = true
			}
		}
		if !valid {
			return p, errPolicy
		}
		for _, existing := range p.private {
			if existing == prefix {
				return p, errPolicy
			}
		}
		p.private = append(p.private, prefix)
	}
	return p, nil
}

func parseURL(raw string) (*url.URL, string, error) {
	if len(raw) > 8192 || strings.ContainsAny(raw, "\\\x00\r\n\t#") {
		return nil, "", errPolicy
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, "", errPolicy
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || len(host) > 253 || strings.HasSuffix(host, ".") || strings.Contains(host, "%") {
		return nil, "", errPolicy
	}
	if a, e := netip.ParseAddr(host); e == nil {
		if a.Is4In6() || a.Zone() != "" {
			return nil, "", errPolicy
		}
		host = a.String()
	} else {
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return nil, "", errPolicy
			}
			for _, ch := range label {
				if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
					return nil, "", errPolicy
				}
			}
		}
	}
	port := u.Port()
	if port == "" {
		if strings.HasSuffix(u.Host, ":") {
			return nil, "", errPolicy
		}
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 {
		return nil, "", errPolicy
	}
	u.Host = net.JoinHostPort(host, strconv.Itoa(n))
	return u, u.Scheme + "://" + u.Host, nil
}

func privateRanges() []netip.Prefix {
	return []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("::1/128")}
}

func (p policy) allowsIP(a netip.Addr) bool {
	if !a.IsValid() || a.Is4In6() || a.Zone() != "" {
		return false
	}
	// Cloud infrastructure service IPs can live inside otherwise private ranges.
	if a == netip.MustParseAddr("168.63.129.16") || a == netip.MustParseAddr("100.100.100.200") || a == netip.MustParseAddr("fd00:ec2::254") {
		return false
	}
	if a.IsPrivate() || a.IsLoopback() {
		for _, prefix := range p.private {
			if prefix.Contains(a) {
				return true
			}
		}
		return false
	}
	if !a.IsGlobalUnicast() {
		return false
	}
	// Conservative exclusions based on IANA special-purpose registries. These
	// intentionally exclude some globally reachable special-use exceptions too.
	var blocked []string
	if a.Is4() {
		blocked = []string{"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4"}
	} else {
		if !netip.MustParsePrefix("2000::/3").Contains(a) {
			return false
		}
		blocked = []string{"2001::/23", "2001:db8::/32", "2002::/16", "3ffe::/16", "3fff::/20"}
	}
	for _, prefix := range blocked {
		if netip.MustParsePrefix(prefix).Contains(a) {
			return false
		}
	}
	return true
}

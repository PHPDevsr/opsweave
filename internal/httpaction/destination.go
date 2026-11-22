package httpaction

import (
	"context"
	"net"
	"net/url"
	"time"
)

// Destination shares the HTTP adapter's immutable origin/IP policy with other
// trusted built-ins. It grants no headers, credentials, redirects or HTTP methods.
type Destination struct {
	origin  string
	address string
	handler handler
}

// NewDestination validates one exact origin and private grants without network I/O.
func NewDestination(origin string, private []string) (*Destination, error) {
	p, err := newPolicy(Config{Enabled: true, AllowedOrigins: []string{origin}, AllowedPrivateCIDRs: private})
	if err != nil {
		return nil, err
	}
	u, canonical, err := parseURL(origin)
	if err != nil {
		return nil, err
	}
	return &Destination{origin: canonical, address: u.Host, handler: handler{policy: p, lookup: net.DefaultResolver.LookupNetIP, dial: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}}, nil
}

// URL normalizes a URL only when it belongs to the configured exact origin.
func (d *Destination) URL(raw string) (*url.URL, error) {
	u, origin, err := parseURL(raw)
	if err != nil || origin != d.origin {
		return nil, errPolicy
	}
	return u, nil
}

// DialContext validates all DNS answers and pins the numeric TCP destination.
func (d *Destination) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" || address != d.address {
		return nil, errPolicy
	}
	return d.handler.connect(ctx, network, address)
}

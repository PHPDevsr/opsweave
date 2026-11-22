package httpaction

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestDestinationExactOriginAndPinnedDNS(t *testing.T) {
	d, err := NewDestination("https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if u, err := d.URL("https://example.com/fixed?secret=synthetic"); err != nil || u.Host != "example.com:443" {
		t.Fatal("canonical origin rejected")
	}
	for _, raw := range []string{"http://example.com/", "https://example.com:444/", "https://evil.example/", "https://user:secret@example.com/", "https://example.com/#fragment"} {
		if _, err := d.URL(raw); err == nil {
			t.Fatal("invalid URL allowed")
		}
	}
	for _, address := range []string{"evil.example:443", "example.com:444"} {
		if _, err := d.DialContext(t.Context(), "tcp", address); err == nil {
			t.Fatal("destination override allowed")
		}
	}
	if _, err := d.DialContext(t.Context(), "udp", "example.com:443"); err == nil {
		t.Fatal("network override allowed")
	}
	sentinel := errors.New("dial observed")
	for _, answers := range [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")},
		{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("169.254.169.254")}, {netip.MustParseAddr("fd00:ec2::254")}, {netip.MustParseAddr("::1")}, {netip.MustParseAddr("::ffff:127.0.0.1")}, nil,
	} {
		calls := 0
		d.handler.lookup = func(context.Context, string, string) ([]netip.Addr, error) { return answers, nil }
		d.handler.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			calls++
			if address != "93.184.216.34:443" {
				t.Error("DNS was not pinned")
			}
			return nil, sentinel
		}
		_, err := d.DialContext(t.Context(), "tcp", "example.com:443")
		if err == nil {
			t.Fatal("missing transport error")
		}
		want := 0
		if len(answers) == 1 && answers[0].String() == "93.184.216.34" {
			want = 1
		}
		if calls != want {
			t.Fatalf("dial count=%d want=%d", calls, want)
		}
	}
	for _, origin := range []string{"https://169.254.169.254", "https://168.63.129.16", "https://[fd00:ec2::254]"} {
		d, err := NewDestination(origin, []string{"10.0.0.0/8", "fd00::/8"})
		if err != nil {
			continue
		}
		d.handler.dial = func(context.Context, string, string) (net.Conn, error) {
			t.Error("metadata address dialed")
			return nil, sentinel
		}
		if _, err := d.DialContext(t.Context(), "tcp", d.address); err == nil {
			t.Fatal("metadata allowed")
		}
	}
}

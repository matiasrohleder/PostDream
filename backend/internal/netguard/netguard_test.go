package netguard

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestIsPublic(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.0.0.5": false, "192.168.1.1": false, "172.16.3.4": false,
		"169.254.169.254": false, "100.64.1.1": false, "0.0.0.0": false, "::1": false,
		"fd00::1": false, "fe80::1": false, "::ffff:127.0.0.1": false, "64:ff9b::a00:1": false,
	}
	for ip, want := range cases {
		if got := IsPublic(netip.MustParseAddr(ip)); got != want {
			t.Errorf("IsPublic(%s)=%v want %v", ip, got, want)
		}
	}
}

func TestPolicyAllowLists(t *testing.T) {
	p := Policy{Allowed: []string{"ci.internal", "*.corp.local", "10.1.0.0/16", "192.168.5.5"}}
	for _, h := range []string{"ci.internal", "CI.internal.", "a.corp.local", "x.y.corp.local"} {
		if !p.hostAllowed(h) {
			t.Errorf("host %s should be allowed", h)
		}
	}
	for _, h := range []string{"corp.local", "evilci.internal", "ci.internal.evil.com"} {
		if p.hostAllowed(h) {
			t.Errorf("host %s should NOT be allowed", h)
		}
	}
	if !p.ipAllowed(netip.MustParseAddr("10.1.2.3")) || !p.ipAllowed(netip.MustParseAddr("192.168.5.5")) {
		t.Error("allowed CIDR/IP rejected")
	}
	if p.ipAllowed(netip.MustParseAddr("10.2.0.1")) {
		t.Error("10.2.0.1 must be blocked")
	}
}

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return f[host], nil
}

func TestDialerBlocksRebinding(t *testing.T) {
	d := &Dialer{Resolver: fakeResolver{"evil.example": {netip.MustParseAddr("127.0.0.1")}}}
	_, err := d.DialContext(context.Background(), "tcp", "evil.example:80")
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
	_, err = d.DialContext(context.Background(), "tcp", "[::1]:80")
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected ErrBlocked for literal IP, got %v", err)
	}
}

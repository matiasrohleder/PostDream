// Package netguard builds HTTP clients for outbound requests whose destination is
// chosen by users (webhooks, link checks). It blocks private, loopback, link-local
// and other internal addresses unless explicitly allowed, and it validates the IP
// *after* DNS resolution and dials that exact IP, so DNS rebinding cannot bypass it.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// ErrBlocked is returned when a destination resolves to a disallowed address.
var ErrBlocked = errors.New("destination address is not allowed")

// Policy decides which destinations are reachable.
type Policy struct {
	// AllowPrivate disables the internal-address check entirely.
	AllowPrivate bool
	// Allowed lists hosts ("ci.internal", "*.corp.local") or CIDRs ("10.1.0.0/16")
	// that may be reached even though they are internal.
	Allowed []string
}

func (p Policy) hostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, a := range p.Allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" || strings.Contains(a, "/") {
			continue
		}
		if a == host || (strings.HasPrefix(a, "*.") && strings.HasSuffix(host, a[1:])) {
			return true
		}
	}
	return false
}

func (p Policy) ipAllowed(ip netip.Addr) bool {
	ip = ip.Unmap()
	if p.AllowPrivate || IsPublic(ip) {
		return true
	}
	for _, a := range p.Allowed {
		if pfx, err := netip.ParsePrefix(strings.TrimSpace(a)); err == nil && pfx.Contains(ip) {
			return true
		}
		if one, err := netip.ParseAddr(strings.TrimSpace(a)); err == nil && one.Unmap() == ip {
			return true
		}
	}
	return false
}

var extraBlocked = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved
	netip.MustParsePrefix("64:ff9b::/96"),  // NAT64 (can embed private IPv4)
	netip.MustParsePrefix("2001:db8::/32"), // documentation
	netip.MustParsePrefix("255.255.255.255/32"),
}

// IsPublic reports whether ip is a globally routable unicast address.
func IsPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range extraBlocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// Resolver is the subset of net.Resolver used (replaceable in tests).
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Dialer dials only to addresses allowed by the policy.
type Dialer struct {
	Policy   Policy
	Resolver Resolver
	Timeout  time.Duration
}

func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		r := d.Resolver
		if r == nil {
			r = net.DefaultResolver
		}
		if ips, err = r.LookupNetIP(ctx, "ip", host); err != nil {
			return nil, err
		}
	}
	explicitlyAllowed := d.Policy.hostAllowed(host)
	var lastErr error = ErrBlocked
	nd := &net.Dialer{Timeout: d.Timeout}
	for _, ip := range ips {
		if !explicitlyAllowed && !d.Policy.ipAllowed(ip) {
			lastErr = fmt.Errorf("%w: %s resolves to %s", ErrBlocked, host, ip)
			continue
		}
		conn, err := nd.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// Client returns an HTTP client that enforces the policy on every connection,
// including the ones made while following redirects. Proxies are never used.
func Client(p Policy, timeout time.Duration) *http.Client {
	d := &Dialer{Policy: p, Timeout: 10 * time.Second}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           d.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("redirect to unsupported scheme")
			}
			return nil
		},
	}
}

// ValidateURL performs cheap static checks before a request is attempted.
func ValidateURL(u string) error {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("URL must start with http:// or https://")
	}
	if len(u) > 2048 {
		return errors.New("URL too long")
	}
	return nil
}

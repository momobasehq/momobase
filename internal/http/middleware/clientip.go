package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP resolves the address a request should be attributed to, which is what
// rate limiting keys on and what request logs record.
type ClientIP func(*http.Request) string

// RemoteClientIP returns the immediate peer's address and ignores every forwarded
// header.
//
// This is the default, and the only safe one without configuration: X-Forwarded-For
// is a header any client can set, so honouring it unconditionally would let a caller
// mint a fresh rate-limit bucket per request and turn the limiter off entirely.
func RemoteClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// NewForwardedClientIP returns a resolver that honours X-Forwarded-For, but only for
// requests arriving from one of the named proxies.
//
// Trust is deliberately directional. The header is read only when the immediate peer
// is itself trusted, and then the chain is walked right to left to the first address
// that is not a trusted proxy — the last hop this deployment did not control. An
// address further left was supplied by something upstream and cannot be believed.
//
// An empty list returns RemoteClientIP, so a deployment that has not opted in behaves
// exactly as it did before.
func NewForwardedClientIP(cidrs []string) (ClientIP, error) {
	var trusted []netip.Prefix
	for _, entry := range cidrs {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// A bare address is accepted as a single-host prefix, since naming one proxy
		// by address is the common case and writing /32 by hand invites a typo.
		if !strings.Contains(entry, "/") {
			address, err := netip.ParseAddr(entry)
			if err != nil {
				return nil, fmt.Errorf("trusted proxy %q is not an IP address or CIDR: %w", entry, err)
			}
			trusted = append(trusted, netip.PrefixFrom(address, address.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q is not a valid CIDR: %w", entry, err)
		}
		trusted = append(trusted, prefix.Masked())
	}
	if len(trusted) == 0 {
		return RemoteClientIP, nil
	}
	return func(r *http.Request) string {
		peer := RemoteClientIP(r)
		address, err := netip.ParseAddr(peer)
		if err != nil || !containsAddr(trusted, address) {
			// The request did not come through a proxy we configured, so whatever it
			// claims about its own origin is unverifiable.
			return peer
		}
		forwarded := r.Header.Get("X-Forwarded-For")
		hops := strings.Split(forwarded, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
			if err != nil {
				// A malformed hop ends the walk: continuing past it would skip over the
				// boundary between what the deployment controls and what it does not.
				return peer
			}
			if !containsAddr(trusted, hop) {
				return hop.String()
			}
		}
		// Every hop was a trusted proxy, so the peer is the closest thing to a client.
		return peer
	}, nil
}

// containsAddr reports whether any prefix covers the address.
func containsAddr(prefixes []netip.Prefix, address netip.Addr) bool {
	// An address arriving as an IPv4-mapped IPv6 value must be compared as IPv4, or a
	// 10.0.0.0/8 prefix would not match ::ffff:10.0.0.1.
	address = address.Unmap()
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

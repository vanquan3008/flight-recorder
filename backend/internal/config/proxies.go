package config

import (
	"net"
	"net/netip"
	"strings"
)

// ParseProxyPrefix reads a trusted proxy entry: a CIDR such as 10.0.0.0/8, or a
// single address, which is taken as a /32 (IPv4) or /128 (IPv6).
func ParseProxyPrefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "/") {
		return netip.ParsePrefix(value)
	}
	address, parseError := netip.ParseAddr(value)
	if parseError != nil {
		return netip.Prefix{}, parseError
	}
	return netip.PrefixFrom(address.Unmap(), address.Unmap().BitLen()), nil
}

// TrustedProxyPrefixes returns the configured trusted proxies. Validation
// rejects bad entries at startup; one that slips through is skipped, which
// fails closed because an unlisted address is never trusted.
func (configuration HTTPConfig) TrustedProxyPrefixes() []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(configuration.TrustedProxies))
	for _, entry := range configuration.TrustedProxies {
		if prefix, parseError := ParseProxyPrefix(entry); parseError == nil {
			prefixes = append(prefixes, prefix.Masked())
		}
	}
	return prefixes
}

// ClientAddress is the address a request is attributed to for rate limits and
// failure budgets. Forwarded headers are written by the client unless a proxy
// overwrites them, so X-Forwarded-For is read only when the TCP peer is one of
// the trusted proxies. It is read from the right: each hop appends the address
// it received from, so skipping trusted proxies leaves the first address a
// trusted party actually saw. A leftmost value is whatever the client chose to
// send and is never used. forwarded holds every X-Forwarded-For header value.
func ClientAddress(remoteAddr string, forwarded []string, trusted []netip.Prefix) string {
	host, _, splitError := net.SplitHostPort(remoteAddr)
	if splitError != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	peer, peerError := netip.ParseAddr(host)
	if peerError != nil {
		if host == "" {
			return "unknown"
		}
		return host
	}
	peer = peer.Unmap()
	if !containsAddress(trusted, peer) {
		return peer.String()
	}
	var hops []string
	for _, header := range forwarded {
		hops = append(hops, strings.Split(header, ",")...)
	}
	for index := len(hops) - 1; index >= 0; index-- {
		hop, hopError := netip.ParseAddr(strings.TrimSpace(hops[index]))
		if hopError != nil {
			return peer.String() // a malformed chain is not evidence of anything
		}
		if hop = hop.Unmap(); !containsAddress(trusted, hop) {
			return hop.String()
		}
	}
	return peer.String()
}

func containsAddress(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

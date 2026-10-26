package config

import (
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

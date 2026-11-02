// Package netaddr holds the one rule for which network addresses outbound
// features, such as webhooks and replays, may connect to.
package netaddr

import "net/netip"

// reserved lists ranges that are not publicly routable although the standard
// library does not classify them as private, loopback or link-local.
var reserved = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT, also used by cloud metadata services
	netip.MustParsePrefix("169.254.0.0/16"),  // link-local, including the cloud metadata address
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation (TEST-NET-1)
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // documentation (TEST-NET-2)
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation (TEST-NET-3)
}

// IsPublic reports whether address is a global unicast address that outbound
// requests may reach. An IPv4 address written in IPv6 form is judged as IPv4,
// so ::ffff:10.0.0.1 is as private as 10.0.0.1.
func IsPublic(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() || address.IsMulticast() {
		return false
	}
	for _, prefix := range reserved {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

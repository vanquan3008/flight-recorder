package netaddr

import "net/netip"

var (
	globalIPv6  = netip.MustParsePrefix("2000::/3")
	blockedIPv4 = [...]netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),       // Current network
		netip.MustParsePrefix("10.0.0.0/8"),      // Private
		netip.MustParsePrefix("100.64.0.0/10"),   // Shared address space
		netip.MustParsePrefix("127.0.0.0/8"),     // Loopback
		netip.MustParsePrefix("169.254.0.0/16"),  // Link-local / metadata
		netip.MustParsePrefix("172.16.0.0/12"),   // Private
		netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
		netip.MustParsePrefix("192.0.2.0/24"),    // Documentation
		netip.MustParsePrefix("192.88.99.0/24"),  // Deprecated 6to4 relay
		netip.MustParsePrefix("192.168.0.0/16"),  // Private
		netip.MustParsePrefix("198.18.0.0/15"),   // Benchmarking
		netip.MustParsePrefix("198.51.100.0/24"), // Documentation
		netip.MustParsePrefix("203.0.113.0/24"),  // Documentation
		netip.MustParsePrefix("224.0.0.0/4"),     // Multicast
		netip.MustParsePrefix("240.0.0.0/4"),     // Reserved / broadcast
	}

	blockedIPv6 = [...]netip.Prefix{
		netip.MustParsePrefix("2001::/23"),     // IETF protocol assignments
		netip.MustParsePrefix("2001:db8::/32"), // Documentation
		netip.MustParsePrefix("2002::/16"),     // 6to4 transition
	}
)

func IsPublic(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" {
		return false
	}
	address = address.Unmap()

	if !address.IsGlobalUnicast() || address.IsPrivate() {
		return false
	}
	if address.Is4() {
		return !contains(blockedIPv4[:], address)
	}
	if !globalIPv6.Contains(address) {
		return false
	}
	return !contains(blockedIPv6[:], address)
}

func contains(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

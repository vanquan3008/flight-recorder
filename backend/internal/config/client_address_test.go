package config

import (
	"net/netip"
	"testing"
)

func TestClientAddressUsesForwardedForOnlyBehindTrustedProxies(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	for _, test := range []struct {
		name      string
		remote    string
		forwarded []string
		trusted   []netip.Prefix
		want      string
	}{
		{"direct client", "198.51.100.7:4000", []string{"1.2.3.4"}, trusted, "198.51.100.7"},
		{"behind a trusted proxy", "10.0.0.5:4000", []string{"203.0.113.9"}, trusted, "203.0.113.9"},
		{"client-chosen leftmost value ignored", "10.0.0.5:4000", []string{"1.2.3.4, 203.0.113.9"}, trusted, "203.0.113.9"},
		{"no proxies configured", "10.0.0.5:4000", []string{"203.0.113.9"}, nil, "10.0.0.5"},
		{"malformed chain falls back to the peer", "10.0.0.5:4000", []string{"203.0.113.9, nope"}, trusted, "10.0.0.5"},
		{"missing address", "", nil, nil, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ClientAddress(test.remote, test.forwarded, test.trusted); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

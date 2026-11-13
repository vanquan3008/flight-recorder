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

func TestEmptyTrustedProxiesFromTheEnvironmentTemplateTrustsNoOne(t *testing.T) {
	setTestEnvironment(t)
	t.Setenv("TRACEPILOT_HTTP_TRUSTED_PROXIES", "")
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatalf(".env.example ships an empty value and must load: %v", loadError)
	}
	if len(configuration.HTTP.TrustedProxyPrefixes()) != 0 {
		t.Fatalf("trusted proxies = %v, want none", configuration.HTTP.TrustedProxies)
	}
}

func TestTrustedProxiesAreReadFromTheEnvironment(t *testing.T) {
	setTestEnvironment(t)
	t.Setenv("TRACEPILOT_HTTP_TRUSTED_PROXIES", "10.0.0.0/8, 192.0.2.7")
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	if got := configuration.HTTP.TrustedProxyPrefixes(); len(got) != 2 || got[0].String() != "10.0.0.0/8" || got[1].String() != "192.0.2.7/32" {
		t.Fatalf("trusted proxies = %v", got)
	}
}

func TestFromTrustedProxyJudgesOnlyTheConnectionPeer(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	for remoteAddr, want := range map[string]bool{
		"10.1.2.3:4567":       true,
		"[::ffff:10.1.2.3]:1": true,
		"203.0.113.9:4567":    false,
		"not-an-address":      false,
		"":                    false,
	} {
		if got := FromTrustedProxy(remoteAddr, trusted); got != want {
			t.Errorf("FromTrustedProxy(%q) = %t, want %t", remoteAddr, got, want)
		}
	}
}

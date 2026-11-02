package config

import (
	"net/netip"
	"testing"
)

func TestEmptyTrustedProxiesFromTheEnvironmentTemplateTrustsNoOne(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
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
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
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
		"10.1.2.3:4567":      true,
		"[::ffff:10.1.2.3]:1": true,
		"203.0.113.9:4567":   false,
		"not-an-address":     false,
		"":                   false,
	} {
		if got := FromTrustedProxy(remoteAddr, trusted); got != want {
			t.Errorf("FromTrustedProxy(%q) = %t, want %t", remoteAddr, got, want)
		}
	}
}

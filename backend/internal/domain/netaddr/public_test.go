package netaddr

import (
	"net/netip"
	"testing"
)

func TestIsPublic(t *testing.T) {
	for value, want := range map[string]bool{
		"8.8.8.8":                true,
		"203.0.114.10":           true,
		"2606:4700:4700::1111":   true,
		"127.0.0.1":              false,
		"10.0.0.1":               false,
		"172.16.0.1":             false,
		"192.168.1.1":            false,
		"169.254.169.254":        false,
		"100.64.0.1":             false,
		"192.0.2.1":              false,
		"198.18.0.1":             false,
		"224.0.0.1":              false,
		"0.0.0.0":                false,
		"::1":                    false,
		"fc00::1":                false,
		"fe80::1":                false,
		"::ffff:10.0.0.1":        false,
		"::ffff:127.0.0.1":       false,
		"::ffff:169.254.169.254": false,
		"::ffff:100.64.0.1":      false,
	} {
		address := netip.MustParseAddr(value)
		if got := IsPublic(address); got != want {
			t.Errorf("IsPublic(%s) = %t, want %t", value, got, want)
		}
	}
}

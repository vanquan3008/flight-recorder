package replay

import (
	"context"
	"net"
	"testing"
)

func TestPolicyRejectsUnsafeTargets(t *testing.T) {
	policy, policyError := NewPolicy([]string{"replay.example.test"}, false)
	if policyError != nil {
		t.Fatal(policyError)
	}
	policy.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	for _, target := range []string{"ftp://replay.example.test", "https://user:password@replay.example.test", "https://other.example.test", "https://replay.example.test"} {
		if _, validateError := policy.Validate(context.Background(), target); validateError == nil {
			t.Fatalf("unsafe target accepted: %s", target)
		}
	}
}

func TestPolicyAllowsExplicitPrivateStagingHost(t *testing.T) {
	policy, policyError := NewPolicy([]string{"staging.internal"}, true)
	if policyError != nil {
		t.Fatal(policyError)
	}
	policy.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.10")}}, nil
	}
	if _, validateError := policy.Validate(context.Background(), "https://staging.internal/replay"); validateError != nil {
		t.Fatal(validateError)
	}
}

func TestPolicyRejectsPrivateDNSResult(t *testing.T) {
	policy, policyError := NewPolicy([]string{"public.example.test"}, false)
	if policyError != nil {
		t.Fatal(policyError)
	}
	policy.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.10")}}, nil
	}
	if _, validateError := policy.Validate(context.Background(), "https://public.example.test/replay"); validateError == nil {
		t.Fatal("expected private DNS result to be rejected")
	}
}

func TestPolicyAllowlistIsHostOnly(t *testing.T) {
	policy, policyError := NewPolicy([]string{"staging.internal"}, true)
	if policyError != nil {
		t.Fatal(policyError)
	}
	policy.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.10")}}, nil
	}
	if _, validateError := policy.Validate(context.Background(), "https://staging.internal:8443/replay"); validateError != nil {
		t.Fatalf("host-only allowlist rejected a different port: %v", validateError)
	}
}

// Carrier-grade NAT space is not public: cloud providers serve metadata from it.
func TestPolicyRejectsSharedAddressSpace(t *testing.T) {
	policy, policyError := NewPolicy([]string{"public.example.test"}, false)
	if policyError != nil {
		t.Fatal(policyError)
	}
	policy.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("100.100.100.200")}}, nil
	}
	if _, validateError := policy.Validate(context.Background(), "https://public.example.test"); validateError == nil {
		t.Fatal("target resolving to shared address space was accepted")
	}
}

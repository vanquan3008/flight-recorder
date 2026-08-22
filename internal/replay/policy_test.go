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

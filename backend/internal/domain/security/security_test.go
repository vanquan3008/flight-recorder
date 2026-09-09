package security

import (
	"testing"
	"time"
)

func TestMatches(t *testing.T) {
	event := Event{SourceIP: "10.12.0.4", Outcome: "failure", Message: "denied", OccurredAt: time.Now(), ID: "event", RawEventID: "raw", Kind: "network"}
	for _, test := range []struct {
		rule Rule
		want bool
	}{
		{Rule{Field: "source_ip", Operator: "cidr", Value: "10.12.0.0/16", Threshold: 1, Window: time.Minute}, true},
		{Rule{Field: "outcome", Operator: "equals", Value: "success", Threshold: 1, Window: time.Minute}, false},
		{Rule{Field: "outcome", Operator: "exists", Threshold: 1, Window: time.Minute}, true},
	} {
		if got := Matches(test.rule, event); got != test.want {
			t.Fatalf("Matches(%+v) = %t", test.rule, got)
		}
	}
}

func TestRuleValidation(t *testing.T) {
	if (Rule{Name: "private source", Field: "source_ip", Operator: "cidr", Value: "10.0.0.0/8", Threshold: 1, Window: time.Minute}).Validate() != nil {
		t.Fatal("valid rule rejected")
	}
	if (Rule{Name: "bad", Field: "anything", Operator: "equals", Value: "x", Threshold: 1, Window: time.Minute}).Validate() == nil {
		t.Fatal("invalid field accepted")
	}
}

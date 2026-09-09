package security

import (
	"testing"
	"time"
)

func TestFromSyslogNormalizesAndKeepsSourceStableAcrossPorts(t *testing.T) {
	timestamp := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	first := FromSyslogWithSourceIdentity("01900000-0000-7000-8000-000000000001", "development", "192.0.2.10:40001", "", "<134>1 2026-08-31T12:00:00Z fw - - - CEF:0|Demo|Firewall|1|42|Denied|8|src=10.0.0.8 dst=203.0.113.5 act=blocked", timestamp)
	second := FromSyslogWithSourceIdentity("01900000-0000-7000-8000-000000000001", "development", "192.0.2.10:40002", "", "<134>1 2026-08-31T12:00:00Z fw - - - CEF:0|Demo|Firewall|1|42|Denied|8|src=10.0.0.8 dst=203.0.113.5 act=blocked", timestamp)
	if first.Security == nil || first.Security.SourceName != "192.0.2.10" || first.Security.SourceID == "" {
		t.Fatalf("unexpected source: %#v", first.Security)
	}
	if first.Security.SourceID != second.Security.SourceID || first.ID != second.ID {
		t.Fatalf("source/event identity is not stable across connection ports")
	}
}

func TestFromSyslogKeepsUnknownRawRecord(t *testing.T) {
	raw := "unrecognized vendor payload"
	event := FromSyslogWithSourceIdentity("01900000-0000-7000-8000-000000000001", "development", "192.0.2.10:40001", "", raw, time.Unix(0, 0).UTC())
	if event.Security != nil || event.LogBody != raw {
		t.Fatalf("unknown record was not preserved as raw telemetry: %#v", event)
	}
}

func TestFromSyslogUsesMTLSSourceIdentity(t *testing.T) {
	event := FromSyslogWithSourceIdentity(
		"01900000-0000-7000-8000-000000000001", "production", "192.0.2.10:40001",
		"firewall-a.example.internal", "CEF:0|Demo|Firewall|1|42|Denied|8|src=10.0.0.8 act=blocked", time.Now().UTC(),
	)
	if event.Security == nil || event.Security.SourceName != "firewall-a.example.internal" {
		t.Fatalf("mTLS source identity was not normalized: %#v", event.Security)
	}
	if event.Attributes["network.peer.address"] != "192.0.2.10:40001" {
		t.Fatalf("peer address was not preserved: %#v", event.Attributes)
	}
}

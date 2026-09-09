package parser

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseCEFWrappedInSyslog(t *testing.T) {
	event, ok := Parse("<134>1 2026-08-31T10:00:00Z fw CEF:0|Acme|Firewall|1|42|Blocked connection|8|src=10.0.0.8 dst=203.0.113.5 spt=1000 dpt=443 act=blocked", time.Now())
	if !ok || event.Format != "syslog+cef" || event.SourceIP != "10.0.0.8" || event.DestinationPort != 443 || event.Severity != "HIGH" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestParseCEFExtensionKeepsSpacesInValue(t *testing.T) {
	event, ok := Parse("CEF:0|Demo|Firewall|1|42|Denied|8|src=10.0.0.8 msg=policy denied by rule act=blocked", time.Now())
	if !ok || event.Message != "policy denied by rule" {
		t.Fatalf("message = %q", event.Message)
	}
}

func FuzzParseNeverPanics(f *testing.F) {
	for _, seed := range []string{
		"", "CEF:0|Demo|Firewall|1|42|Denied|8|src=10.0.0.8", "LEEF:2.0|Demo|Firewall|1|42|src=10.0.0.8",
		"<134>1 2026-08-31T10:00:00Z fw - - - message", `{"kind":"login","src":"192.0.2.1"}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		_, _ = Parse(raw, time.Unix(0, 0).UTC())
	})
}

func TestParseJSON(t *testing.T) {
	event, ok := Parse(`{"event_kind":"authentication","src":"192.0.2.10","outcome":"failure"}`, time.Now())
	if !ok || event.Kind != "authentication" || event.SourceIP != "192.0.2.10" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestParseJSONApplicationLogStaysTelemetryOnly(t *testing.T) {
	if _, ok := Parse(`{"message":"request completed","level":"info"}`, time.Now()); ok {
		t.Fatal("generic application JSON was recognized as a security event")
	}
}

func TestParseShortRFC5424NeverPanics(t *testing.T) {
	if _, ok := Parse("<0>1 0 0 0", time.Unix(0, 0).UTC()); !ok {
		t.Fatal("short syslog frame should remain queryable as a raw syslog event")
	}
}

func TestParseMalformedJSONFallsBackToRaw(t *testing.T) {
	if _, ok := Parse(`{"kind":"login",`, time.Unix(0, 0).UTC()); ok {
		t.Fatal("malformed JSON was recognized")
	}
}

func TestParseSanitizedAcmeFirewallFixture(t *testing.T) {
	raw, readError := os.ReadFile("testdata/acme_firewall.cef")
	if readError != nil {
		t.Fatal(readError)
	}
	event, ok := Parse(string(raw), time.Unix(0, 0).UTC())
	if !ok || event.Format != "cef" || event.ObserverVendor != "Acme" || event.Action != "blocked" || event.Outcome != "failure" || event.SourcePort != 51324 {
		t.Fatalf("fixture event = %#v", event)
	}
}

func TestParseSyslogNormalizesTimestampAndRejectsInvalidPriority(t *testing.T) {
	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	event, ok := Parse("<134>1 2026-08-31T10:00:00+02:00 fw - - - login failed", base)
	if !ok || !event.OccurredAt.Equal(time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)) || event.HostName != "fw" || event.Message != "login failed" {
		t.Fatalf("unexpected RFC5424 event: %#v", event)
	}
	if _, ok := Parse("<999> malformed", base); ok {
		t.Fatal("accepted out-of-range syslog priority")
	}
	if _, ok := Parse("<x> malformed", base); ok {
		t.Fatal("accepted non-numeric syslog priority")
	}
}

func TestParseRFC3164AndLEEFDelimiter(t *testing.T) {
	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	event, ok := Parse("<13>Aug 31 11:59:58 router sshd[1]: login failed", base)
	if !ok || event.Format != "syslog" || event.HostName != "router" || event.Message != "sshd[1]: login failed" || !event.OccurredAt.Equal(time.Date(2026, 8, 31, 11, 59, 58, 0, time.UTC)) {
		t.Fatalf("unexpected RFC3164 event: %#v", event)
	}
	event, ok = Parse("LEEF:2.0|Acme|Firewall|1|42|^|eventKind=login^src=192.0.2.1^msg=denied", base)
	if !ok || event.Format != "leef" || event.Kind != "login" || event.SourceIP != "192.0.2.1" || event.Message != "denied" {
		t.Fatalf("unexpected LEEF event: %#v", event)
	}
}

func TestParseOversizedAndInvalidUTF8NeverPanics(t *testing.T) {
	if _, ok := Parse("CEF:0|Acme|Firewall|1|42|Denied|8|msg="+strings.Repeat("x", 1<<20), time.Unix(0, 0).UTC()); !ok {
		t.Fatal("valid oversized field should remain parseable")
	}
	if _, ok := Parse(string([]byte{'C', 'E', 'F', ':', 0xff}), time.Unix(0, 0).UTC()); ok {
		t.Fatal("invalid UTF-8 CEF should be rejected")
	}
}

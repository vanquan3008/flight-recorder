package telemetry

import "testing"

func TestValidUUIDAcceptsOnlyCanonicalText(t *testing.T) {
	for _, value := range []string{"01900000-0000-7000-8000-000000000001", "0190ABCD-0000-7000-8000-00000000000f"} {
		if !ValidUUID(value) {
			t.Errorf("rejected %q", value)
		}
	}
	for _, value := range []string{"", "not-a-uuid", "01900000-0000-7000-8000-00000000000z", "0190000000007000800000000000000100000", "01900000_0000-7000-8000-000000000001", "01900000-0000-7000-8000-0000000000012"} {
		if ValidUUID(value) {
			t.Errorf("accepted %q", value)
		}
	}
}

package logs

import "testing"

func TestLogSearchPatternMatchesLiterallyIgnoringCase(t *testing.T) {
	for term, want := range map[string]string{
		"Timeout":    `%timeout%`,
		`say "hi"`:   `%say "hi"%`,
		"line\nnext": "%line\nnext%",
		"50%_off":    `%50\%\_off%`,
		`C:\temp`:    `%c:\\temp%`,
		"Đặt hàng":   `%đặt hàng%`,
	} {
		if got := logSearchPattern(term); got != want {
			t.Errorf("logSearchPattern(%q) = %q, want %q", term, got, want)
		}
	}
}

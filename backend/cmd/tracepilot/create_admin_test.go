package main

import (
	"strings"
	"testing"
)

func TestParseCreateAdmin(t *testing.T) {
	valid := []string{"--email", " Admin@Tracepilot.test ", "--name", " Operator ", "--password-stdin"}
	for _, test := range []struct {
		name, password string
		args           []string
		valid          bool
	}{
		{"valid", "strong-operator-password\r\n", valid, true},
		{"preserves spaces", "  strong-operator-password  \n", valid, true},
		{"weak", "short", valid, false},
		{"empty", "", valid, false},
		{"multiple lines", "strong-operator-password\nsecond", valid, false},
		{"oversized input", strings.Repeat("secret", 1000), valid, false},
		{"stdin flag required", "strong-operator-password", valid[:4], false},
		{"unknown flag", "strong-operator-password", append(append([]string{}, valid...), "--password=do-not-log-this"), false},
		{"no positional args", "strong-operator-password", append(append([]string{}, valid...), "extra"), false},
		{"invalid email", "strong-operator-password", []string{"--email", "invalid", "--name", "Operator", "--password-stdin"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			params, err := parseCreateAdmin(test.args, strings.NewReader(test.password))
			if (err == nil) != test.valid {
				t.Fatalf("error=%v, want valid=%t", err, test.valid)
			}
			if err != nil {
				if strings.Contains(err.Error(), "do-not-log-this") || strings.Contains(err.Error(), test.password) && test.password != "" {
					t.Fatal("validation error leaked password")
				}
				return
			}
			want := strings.TrimSuffix(strings.TrimSuffix(test.password, "\n"), "\r")
			if params.Email != "admin@tracepilot.test" || params.DisplayName != "Operator" || params.Password != want || !params.PlatformAdmin || params.MustChangePassword {
				t.Fatal("administrator fields or password were not preserved")
			}
		})
	}
}

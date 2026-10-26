package config

import "testing"

func TestValidateRejectsResourceExhaustingQueue(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Ingestion.QueueSize = maxIngestionQueueSize + 1
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected resource-exhausting queue size to fail")
	}
}

func TestValidateRejectsShortBootstrapAPIKey(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Auth.BootstrapAPIKey = "too-short"
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected short bootstrap key to fail")
	}
}

func TestValidateRequiresReplayAllowlist(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Replay.Enabled = true
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected enabled replay without allowlist to fail")
	}
}

func TestValidateRequiresCompleteTLSConfiguration(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://tracepilot:tracepilot@localhost/tracepilot"
	configuration.TLS.CertificateFile = "certificate.pem"
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected incomplete TLS configuration to fail")
	}
}

func TestValidateProductionRequiresAuthAndTLS(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://tracepilot:tracepilot@localhost/tracepilot?sslmode=require"
	configuration.Production = true
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected production authentication and TLS to be required")
	}
	configuration.Auth.BootstrapAPIKey = "production-bootstrap-key-000000000000000000000000000000"
	configuration.TLS.CertificateFile = "certificate.pem"
	configuration.TLS.PrivateKeyFile = "private-key.pem"
	if err := configuration.Validate(); err != nil {
		t.Fatalf("production configuration rejected: %v", err)
	}
}

func TestValidateRejectsInvalidAllowedOrigin(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.HTTP.AllowedOrigins = []string{"https://operator.example.test/path"}
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected invalid allowed origin to fail")
	}
}

func TestValidateProductionRequiresVerifiedDatabaseTransport(t *testing.T) {
	cases := []struct {
		name, url string
		wantErr   bool
	}{
		{"missing sslmode", "postgres://u:p@db.internal/tracepilot", true},
		{"disable", "postgres://u:p@db.internal/tracepilot?sslmode=disable", true},
		{"prefer", "postgres://u:p@db.internal/tracepilot?sslmode=prefer", true},
		{"keyword dsn", "host=db.internal sslmode=disable", true},
		{"require", "postgres://u:p@db.internal/tracepilot?sslmode=require", false},
		{"verify-full", "postgresql://u:p@db.internal/tracepilot?sslmode=verify-full", false},
		{"unix socket", "postgres://u:p@/tracepilot?host=/var/run/postgresql", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			configuration := defaultConfig()
			configuration.Production = true
			configuration.Database.URL = testCase.url
			configuration.Auth.BootstrapAPIKey = "production-bootstrap-key-000000000000000000000000000000"
			configuration.TLS.CertificateFile = "certificate.pem"
			configuration.TLS.PrivateKeyFile = "private-key.pem"
			if err := configuration.Validate(); (err != nil) != testCase.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, testCase.wantErr)
			}
		})
	}
}

func TestValidateRejectsInvalidConcurrentRequestLimit(t *testing.T) {
	for _, limit := range []int{0, -1, maxIngestionConcurrentRequests + 1} {
		configuration := defaultConfig()
		configuration.Database.URL = "postgres://test"
		configuration.Ingestion.MaxConcurrentRequests = limit
		if err := configuration.Validate(); err == nil {
			t.Fatalf("expected max_concurrent_requests=%d to fail", limit)
		}
	}
}

func TestValidateTrustedProxies(t *testing.T) {
	for entry, valid := range map[string]bool{
		"10.0.0.0/8": true, "192.0.2.1": true, "fd00::/8": true, " 172.16.0.0/12 ": true,
		"": false, "not-an-ip": false, "10.0.0.0/33": false, "*": false,
	} {
		configuration := defaultConfig()
		configuration.Database.URL = "postgres://test"
		configuration.HTTP.TrustedProxies = []string{entry}
		if err := configuration.Validate(); (err == nil) != valid {
			t.Fatalf("trusted proxy %q: valid=%t, error=%v", entry, valid, err)
		}
	}
}

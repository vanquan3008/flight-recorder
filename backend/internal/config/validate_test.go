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

func TestValidateDatabaseSessionLimits(t *testing.T) {
	valid := defaultConfig()
	valid.Database.URL = "postgres://test"
	if err := valid.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	for name, change := range map[string]func(*Config){
		"statement timeout below the API request timeout": func(c *Config) { c.Database.StatementTimeout = c.HTTP.RequestTimeout - 1 },
		"negative lock timeout":                           func(c *Config) { c.Database.LockTimeout = -1 },
		"negative idle in transaction timeout":            func(c *Config) { c.Database.IdleInTransactionTimeout = -1 },
	} {
		configuration := valid
		change(&configuration)
		if err := configuration.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	off := valid
	off.Database.StatementTimeout, off.Database.LockTimeout, off.Database.IdleInTransactionTimeout = 0, 0, 0
	if err := off.Validate(); err != nil {
		t.Fatalf("0 must keep the server defaults: %v", err)
	}
}

func TestValidateOTLPAuthFailureLimit(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Ingestion.AuthFailureLimit = -1
	if err := configuration.Validate(); err == nil {
		t.Fatal("negative limit accepted")
	}
	configuration.Ingestion.AuthFailureLimit = 5
	configuration.Ingestion.AuthFailureWindow = 0
	if err := configuration.Validate(); err == nil {
		t.Fatal("a limit without a window was accepted")
	}
	configuration.Ingestion.AuthFailureLimit = 0
	if err := configuration.Validate(); err != nil {
		t.Fatalf("limit 0 turns the guard off: %v", err)
	}
}

func TestValidateProductionRejectsSampleSecrets(t *testing.T) {
	production := func() Config {
		configuration := defaultConfig()
		configuration.Production = true
		configuration.Database.URL = "postgres://tracepilot:s3cure-random-pw@db.internal/tracepilot?sslmode=verify-full"
		configuration.Auth.BootstrapAPIKey = "b0f3a1c9e7d24a6f8c15e2d97a40b3c6f1e8d205"
		configuration.TLS.CertificateFile = "certificate.pem"
		configuration.TLS.PrivateKeyFile = "private-key.pem"
		return configuration
	}
	if err := production().Validate(); err != nil {
		t.Fatalf("a real production configuration was rejected: %v", err)
	}
	for name, change := range map[string]func(*Config){
		"bootstrap key from .env.example": func(c *Config) { c.Auth.BootstrapAPIKey = "tracepilot-development-bootstrap-key-000000" },
		"bootstrap key with a marker":     func(c *Config) { c.Auth.BootstrapAPIKey = "my-ChangeMe-key-0000000000000000000000000" },
		"database password from the sample": func(c *Config) {
			c.Database.URL = "postgres://tracepilot:tracepilot-dev-only@db.internal/tracepilot?sslmode=verify-full"
		},
	} {
		configuration := production()
		change(&configuration)
		if err := configuration.Validate(); err == nil {
			t.Errorf("%s was accepted in production", name)
		}
	}
	development := defaultConfig()
	development.Database.URL = "postgres://tracepilot:tracepilot-dev-only@localhost/tracepilot?sslmode=disable"
	development.Auth.BootstrapAPIKey = "tracepilot-development-bootstrap-key-000000"
	if err := development.Validate(); err != nil {
		t.Fatalf("the sample secrets must keep working outside production: %v", err)
	}
}

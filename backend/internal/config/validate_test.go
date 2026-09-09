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
	configuration.Database.URL = "postgres://tracepilot:tracepilot@localhost/tracepilot"
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

func TestValidateProductionSecurityRequiresTLS(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://tracepilot:tracepilot@localhost/tracepilot"
	configuration.Production = true
	configuration.Auth.BootstrapAPIKey = "production-bootstrap-key-000000000000000000000000000000"
	configuration.TLS.CertificateFile = "certificate.pem"
	configuration.TLS.PrivateKeyFile = "private-key.pem"
	configuration.Security.Enabled = true
	configuration.Security.SyslogAddress = ":6514"
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected plaintext production syslog to fail")
	}
	configuration.Security.SyslogTLS = true
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected production TLS syslog without a client CA to fail")
	}
	configuration.Security.SyslogClientCAFile = "ca.pem"
	if err := configuration.Validate(); err != nil {
		t.Fatalf("production security configuration rejected: %v", err)
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

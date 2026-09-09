package config

import "os"

func applyEnvironment(configuration *Config) error {
	var parseError error
	configuration.HTTP.Address = environmentValue("TRACEPILOT_HTTP_ADDRESS", configuration.HTTP.Address)
	if allowedOrigins, exists := os.LookupEnv("TRACEPILOT_HTTP_ALLOWED_ORIGINS"); exists {
		configuration.HTTP.AllowedOrigins = splitCommaSeparated(allowedOrigins)
	}
	configuration.Ingestion.GRPCAddress = environmentValue("TRACEPILOT_INGESTION_GRPC_ADDRESS", configuration.Ingestion.GRPCAddress)
	configuration.Ingestion.HTTPAddress = environmentValue("TRACEPILOT_INGESTION_HTTP_ADDRESS", configuration.Ingestion.HTTPAddress)
	configuration.Ingestion.ProjectID = environmentValue("TRACEPILOT_INGESTION_PROJECT_ID", configuration.Ingestion.ProjectID)
	configuration.Ingestion.ProjectSlug = environmentValue("TRACEPILOT_INGESTION_PROJECT_SLUG", configuration.Ingestion.ProjectSlug)
	configuration.Ingestion.ProjectName = environmentValue("TRACEPILOT_INGESTION_PROJECT_NAME", configuration.Ingestion.ProjectName)
	configuration.Ingestion.EnvironmentID = environmentValue("TRACEPILOT_INGESTION_ENVIRONMENT_ID", configuration.Ingestion.EnvironmentID)
	configuration.Ingestion.EnvironmentName = environmentValue("TRACEPILOT_INGESTION_ENVIRONMENT_NAME", configuration.Ingestion.EnvironmentName)
	configuration.Database.URL = environmentValue("TRACEPILOT_DATABASE_URL", configuration.Database.URL)
	configuration.Database.MigrationsPath = environmentValue("TRACEPILOT_MIGRATIONS_PATH", configuration.Database.MigrationsPath)
	configuration.Auth.BootstrapAPIKey = environmentValue("TRACEPILOT_AUTH_BOOTSTRAP_API_KEY", configuration.Auth.BootstrapAPIKey)
	configuration.TLS.CertificateFile = environmentValue("TRACEPILOT_TLS_CERTIFICATE_FILE", configuration.TLS.CertificateFile)
	configuration.TLS.PrivateKeyFile = environmentValue("TRACEPILOT_TLS_PRIVATE_KEY_FILE", configuration.TLS.PrivateKeyFile)
	if configuration.Production, parseError = readBoolean("TRACEPILOT_PRODUCTION", configuration.Production); parseError != nil {
		return parseError
	}
	if configuration.Replay.Enabled, parseError = readBoolean("TRACEPILOT_REPLAY_ENABLED", configuration.Replay.Enabled); parseError != nil {
		return parseError
	}
	if configuration.Security.Enabled, parseError = readBoolean("TRACEPILOT_SECURITY_ENABLED", configuration.Security.Enabled); parseError != nil {
		return parseError
	}
	if configuration.Notifications.Enabled, parseError = readBoolean("TRACEPILOT_NOTIFICATIONS_ENABLED", configuration.Notifications.Enabled); parseError != nil {
		return parseError
	}
	configuration.Notifications.EncryptionKey = environmentValue("TRACEPILOT_WEBHOOK_ENCRYPTION_KEY", configuration.Notifications.EncryptionKey)
	configuration.Security.SyslogAddress = environmentValue("TRACEPILOT_SECURITY_SYSLOG_ADDRESS", configuration.Security.SyslogAddress)
	configuration.Security.SyslogClientCAFile = environmentValue("TRACEPILOT_SECURITY_SYSLOG_CLIENT_CA_FILE", configuration.Security.SyslogClientCAFile)
	if configuration.Security.SyslogTLS, parseError = readBoolean("TRACEPILOT_SECURITY_SYSLOG_TLS", configuration.Security.SyslogTLS); parseError != nil {
		return parseError
	}
	if replayAllowedHosts, exists := os.LookupEnv("TRACEPILOT_REPLAY_ALLOWED_HOSTS"); exists {
		configuration.Replay.AllowedHosts = splitCommaSeparated(replayAllowedHosts)
	}
	if configuration.Replay.AllowPrivate, parseError = readBoolean("TRACEPILOT_REPLAY_ALLOW_PRIVATE", configuration.Replay.AllowPrivate); parseError != nil {
		return parseError
	}
	if configuration.Retention.MaxAge, parseError = readNonNegativeDuration("TRACEPILOT_RETENTION_MAX_AGE", configuration.Retention.MaxAge); parseError != nil {
		return parseError
	}

	if configuration.HTTP.ReadHeaderTimeout, parseError = readDuration("TRACEPILOT_HTTP_READ_HEADER_TIMEOUT", configuration.HTTP.ReadHeaderTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.ReadTimeout, parseError = readDuration("TRACEPILOT_HTTP_READ_TIMEOUT", configuration.HTTP.ReadTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.WriteTimeout, parseError = readDuration("TRACEPILOT_HTTP_WRITE_TIMEOUT", configuration.HTTP.WriteTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.IdleTimeout, parseError = readDuration("TRACEPILOT_HTTP_IDLE_TIMEOUT", configuration.HTTP.IdleTimeout); parseError != nil {
		return parseError
	}
	if configuration.ShutdownTimeout, parseError = readDuration("TRACEPILOT_SHUTDOWN_TIMEOUT", configuration.ShutdownTimeout); parseError != nil {
		return parseError
	}
	if configuration.Replay.Timeout, parseError = readDuration("TRACEPILOT_REPLAY_TIMEOUT", configuration.Replay.Timeout); parseError != nil {
		return parseError
	}
	if configuration.Retention.CheckInterval, parseError = readDuration("TRACEPILOT_RETENTION_CHECK_INTERVAL", configuration.Retention.CheckInterval); parseError != nil {
		return parseError
	}
	if configuration.HTTP.MaxHeaderBytes, parseError = readPositiveInteger("TRACEPILOT_HTTP_MAX_HEADER_BYTES", configuration.HTTP.MaxHeaderBytes); parseError != nil {
		return parseError
	}
	if configuration.HTTP.ManagementMaxBodyBytes, parseError = readPositiveInt64("TRACEPILOT_HTTP_MANAGEMENT_MAX_BODY_BYTES", configuration.HTTP.ManagementMaxBodyBytes); parseError != nil {
		return parseError
	}
	if configuration.HTTP.RateLimitRequests, parseError = readPositiveInteger("TRACEPILOT_HTTP_RATE_LIMIT_REQUESTS", configuration.HTTP.RateLimitRequests); parseError != nil {
		return parseError
	}
	if configuration.HTTP.RateLimitWindow, parseError = readDuration("TRACEPILOT_HTTP_RATE_LIMIT_WINDOW", configuration.HTTP.RateLimitWindow); parseError != nil {
		return parseError
	}
	if configuration.HTTP.RateLimitAuthRequests, parseError = readPositiveInteger("TRACEPILOT_HTTP_RATE_LIMIT_AUTH_REQUESTS", configuration.HTTP.RateLimitAuthRequests); parseError != nil {
		return parseError
	}
	if configuration.HTTP.RateLimitReplayRequests, parseError = readPositiveInteger("TRACEPILOT_HTTP_RATE_LIMIT_REPLAY_REQUESTS", configuration.HTTP.RateLimitReplayRequests); parseError != nil {
		return parseError
	}
	if configuration.Replay.MaxResponseBytes, parseError = readPositiveInt64("TRACEPILOT_REPLAY_MAX_RESPONSE_BYTES", configuration.Replay.MaxResponseBytes); parseError != nil {
		return parseError
	}
	if configuration.Replay.MaxRequestBytes, parseError = readPositiveInt64("TRACEPILOT_REPLAY_MAX_REQUEST_BYTES", configuration.Replay.MaxRequestBytes); parseError != nil {
		return parseError
	}
	if configuration.Replay.MaxConcurrent, parseError = readPositiveInteger("TRACEPILOT_REPLAY_MAX_CONCURRENT", configuration.Replay.MaxConcurrent); parseError != nil {
		return parseError
	}
	if configuration.Retention.BatchSize, parseError = readPositiveInteger("TRACEPILOT_RETENTION_BATCH_SIZE", configuration.Retention.BatchSize); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxRequestBytes, parseError = readPositiveInt64("TRACEPILOT_INGESTION_MAX_REQUEST_BYTES", configuration.Ingestion.MaxRequestBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxRecordsPerRequest, parseError = readPositiveInteger("TRACEPILOT_INGESTION_MAX_RECORDS_PER_REQUEST", configuration.Ingestion.MaxRecordsPerRequest); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributesPerRecord, parseError = readPositiveInteger("TRACEPILOT_INGESTION_MAX_ATTRIBUTES_PER_RECORD", configuration.Ingestion.MaxAttributesPerRecord); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributeValueBytes, parseError = readPositiveInteger("TRACEPILOT_INGESTION_MAX_ATTRIBUTE_VALUE_BYTES", configuration.Ingestion.MaxAttributeValueBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributeDepth, parseError = readPositiveInteger("TRACEPILOT_INGESTION_MAX_ATTRIBUTE_DEPTH", configuration.Ingestion.MaxAttributeDepth); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.QueueSize, parseError = readPositiveInteger("TRACEPILOT_INGESTION_QUEUE_SIZE", configuration.Ingestion.QueueSize); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.QueueMaxBytes, parseError = readPositiveInt64("TRACEPILOT_INGESTION_QUEUE_MAX_BYTES", configuration.Ingestion.QueueMaxBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.EnqueueTimeout, parseError = readDuration("TRACEPILOT_INGESTION_ENQUEUE_TIMEOUT", configuration.Ingestion.EnqueueTimeout); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.BatchSize, parseError = readPositiveInteger("TRACEPILOT_INGESTION_BATCH_SIZE", configuration.Ingestion.BatchSize); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.BatchTimeout, parseError = readDuration("TRACEPILOT_INGESTION_BATCH_TIMEOUT", configuration.Ingestion.BatchTimeout); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.FlushInterval, parseError = readDuration("TRACEPILOT_INGESTION_FLUSH_INTERVAL", configuration.Ingestion.FlushInterval); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.Workers, parseError = readPositiveInteger("TRACEPILOT_INGESTION_WORKERS", configuration.Ingestion.Workers); parseError != nil {
		return parseError
	}
	if configuration.Security.MaxLineBytes, parseError = readPositiveInteger("TRACEPILOT_SECURITY_MAX_LINE_BYTES", configuration.Security.MaxLineBytes); parseError != nil {
		return parseError
	}
	if configuration.Security.MaxConnections, parseError = readPositiveInteger("TRACEPILOT_SECURITY_MAX_CONNECTIONS", configuration.Security.MaxConnections); parseError != nil {
		return parseError
	}
	if configuration.Security.IdleTimeout, parseError = readDuration("TRACEPILOT_SECURITY_IDLE_TIMEOUT", configuration.Security.IdleTimeout); parseError != nil {
		return parseError
	}
	if configuration.Security.DetectionInterval, parseError = readDuration("TRACEPILOT_SECURITY_DETECTION_INTERVAL", configuration.Security.DetectionInterval); parseError != nil {
		return parseError
	}
	if configuration.Notifications.PollInterval, parseError = readDuration("TRACEPILOT_WEBHOOK_POLL_INTERVAL", configuration.Notifications.PollInterval); parseError != nil {
		return parseError
	}
	if configuration.Notifications.RequestTimeout, parseError = readDuration("TRACEPILOT_WEBHOOK_REQUEST_TIMEOUT", configuration.Notifications.RequestTimeout); parseError != nil {
		return parseError
	}
	if configuration.Notifications.Workers, parseError = readPositiveInteger("TRACEPILOT_WEBHOOK_WORKERS", configuration.Notifications.Workers); parseError != nil {
		return parseError
	}
	if redactedKeys, exists := os.LookupEnv("TRACEPILOT_INGESTION_REDACTED_KEYS"); exists {
		configuration.Ingestion.RedactedKeys = splitCommaSeparated(redactedKeys)
	}
	if configuration.Database.MaxConnections, parseError = readPositiveInt32("TRACEPILOT_DATABASE_MAX_CONNECTIONS", configuration.Database.MaxConnections); parseError != nil {
		return parseError
	}
	if configuration.Database.MinConnections, parseError = readNonNegativeInt32("TRACEPILOT_DATABASE_MIN_CONNECTIONS", configuration.Database.MinConnections); parseError != nil {
		return parseError
	}
	if configuration.Database.MaxConnectionLifetime, parseError = readDuration("TRACEPILOT_DATABASE_MAX_CONNECTION_LIFETIME", configuration.Database.MaxConnectionLifetime); parseError != nil {
		return parseError
	}
	if configuration.Database.MaxConnectionIdleTime, parseError = readDuration("TRACEPILOT_DATABASE_MAX_CONNECTION_IDLE_TIME", configuration.Database.MaxConnectionIdleTime); parseError != nil {
		return parseError
	}
	if configuration.Database.HealthCheckPeriod, parseError = readDuration("TRACEPILOT_DATABASE_HEALTH_CHECK_PERIOD", configuration.Database.HealthCheckPeriod); parseError != nil {
		return parseError
	}
	if configuration.Database.ReadinessTimeout, parseError = readDuration("TRACEPILOT_DATABASE_READINESS_TIMEOUT", configuration.Database.ReadinessTimeout); parseError != nil {
		return parseError
	}
	if configuration.LogLevel, parseError = parseLogLevel(environmentValue("TRACEPILOT_LOG_LEVEL", configuration.LogLevel.String())); parseError != nil {
		return parseError
	}
	return nil
}

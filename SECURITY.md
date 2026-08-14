# Security policy

Do not open a public issue for a suspected vulnerability involving credential exposure, cross-project access, replay SSRF, or arbitrary file access. Use the repository's private vulnerability reporting feature. The repository owner must publish a monitored security contact before the first release.

Flight is pre-release and is not yet approved for ingesting production telemetry. Phase 1 provides strict configuration, bounded management HTTP, database/schema readiness, non-root containers, explicit migrations, and dependency scanning. OTLP authentication/limits, payload capture/redaction, management authentication, and replay remain disabled until their security gates pass. See `PLAN.md` and `docs/security.md`.

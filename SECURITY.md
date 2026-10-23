# Security policy

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability involving credential
exposure, cross-project access, replay SSRF, webhook abuse, or arbitrary file
access. Use the repository's private vulnerability reporting feature and include
the affected version or commit, reproduction steps with synthetic data only, and
the impact you observed. The repository owner must publish a monitored security
contact before the first release.

## Supported versions

TracePilot is in beta. Only the latest tagged release and `main` receive fixes.

## Deployment baseline

TracePilot is not approved for production telemetry until the owner has accepted
the release gates in `docs/OPERATIONS.md`. Production deployments must:

- enable authentication, TLS on management/OTLP listeners and PostgreSQL
  `verify-full`, and replace the development bootstrap key;
- use least-privilege API keys (`ingest` for applications, `read` for daily
  investigation, `admin` only for key and webhook management) and revoke the
  bootstrap key once an admin key exists;
- keep replay disabled unless its host allowlist has been reviewed;
- keep webhook endpoints HTTPS-only with the encryption key stored in a secret
  manager, and never log API keys, webhook secrets or raw telemetry.

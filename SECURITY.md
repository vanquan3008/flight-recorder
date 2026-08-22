# Security policy

Do not open a public issue for a suspected vulnerability involving credential exposure, cross-project access, replay SSRF, or arbitrary file access. Use the repository's private vulnerability reporting feature. The repository owner must publish a monitored security contact before the first release.

Flight is pre-release and is not yet approved for ingesting production telemetry. Production deployments must enable authentication and TLS, keep replay disabled unless its host allowlist is reviewed, and complete the release gates in `docs/OPERATIONS.md`.

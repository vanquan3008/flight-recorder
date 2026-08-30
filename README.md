# TracePilot

TracePilot is a release-candidate, self-hosted OpenTelemetry trace and log backend. It receives OTLP data, redacts configured sensitive attributes, applies bounded batch persistence, and stores canonical events plus first-class spans/logs in PostgreSQL.

The management API is served under `/api/v1`.

With a bootstrap API key configured, it provides project-scoped trace/log search, service and incident APIs. Optional allowed request execution is disabled by default; it sends a new, redacted request only to explicit allowlisted hosts and records metadata for every attempt. It does not replay captured traffic. A file log agent is included for shipping local log files.

## Architecture Overview

```mermaid
flowchart LR
    App[Application] --> OTel[OpenTelemetry SDK / Collector]
    OTel --> Receiver[OTLP gRPC/HTTP]
    Receiver --> Normalize[Normalize & Redact]
    Normalize --> Queue[Bounded Queue]
    Queue --> Batch[Batch Workers]
    Batch -->|Commit| PG[(PostgreSQL)]
    PG --> Health[Health and Prometheus metrics]
    PG --> API[Authenticated query API]
```

## Repository Structure

```text
backend/       Go API, OTLP ingestion, migrations, and Docker image (see backend/README.md)
scripts/      backend integration and Compose smoke checks
```

## Configuration

TracePilot uses compiled defaults, then an optional versioned YAML file, then environment overrides. Create `.env` only when overriding defaults:

```env
POSTGRES_PASSWORD=replace-with-a-local-password
# Database/user identifiers stay stable so existing data volumes remain readable.
TRACEPILOT_DATABASE_URL=postgres://flight:replace-with-a-local-password@postgres:5432/flight?sslmode=disable
TRACEPILOT_AUTH_BOOTSTRAP_API_KEY=replace-with-a-secret-at-least-32-characters
```

For native development, change the database host from `postgres` to `localhost`.
The backend loads `.env` from either `backend/` or the repository root.

For production, set `TRACEPILOT_PRODUCTION=true`, provide a bootstrap API key with at least 32 characters, and configure both `TRACEPILOT_TLS_CERTIFICATE_FILE` and `TRACEPILOT_TLS_PRIVATE_KEY_FILE`. The service refuses to start in production when authentication or TLS is incomplete.

Replay allowlists are host-based (all ports of an allowed host); private-address targets remain blocked unless `allow_private` is explicitly enabled.

## How to Run

TracePilot provides a `Makefile` with shortcuts to run the project effortlessly. You can choose to run it entirely in Docker or natively with Go.

### Docker Compose (recommended)
This is the fastest way to get everything running, including the database and migrations.

```bash
make dev
```
This starts PostgreSQL, applies migrations, then starts the TracePilot API.
Verify it at `http://localhost:8080/health/ready`.

### Native development
If you prefer running the Go code locally for easier debugging:

1. Start the database:

```bash
docker compose up -d postgres
```

2. Run migrations:

```bash
make migrate
```

3. Start the API:

```bash
make run
```

## License

TracePilot is licensed under the Apache License 2.0.

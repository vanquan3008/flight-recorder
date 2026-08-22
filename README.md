# Flight

Flight is an early-stage, self-hosted OpenTelemetry trace and log service. It receives OTLP data, redacts configured sensitive attributes, applies bounded batch persistence, and stores canonical events plus first-class spans/logs in PostgreSQL.

With a bootstrap API key configured, it provides project-scoped trace/log search, service and incident views, and a minimal browser UI. Optional replay is disabled by default; it forwards only redacted JSON bodies and an allowlisted set of non-secret headers to explicit allowlisted hosts, then records every attempt. A file log agent is included for shipping local log files.

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
    PG --> API[Authenticated query API and UI]
```

## Repository Structure

```text
cmd/          binaries
docs/         operations documentation
internal/     application packages
migrations/   PostgreSQL migrations
```

## Configuration

Flight uses compiled defaults, then an optional versioned YAML file, then environment overrides. Create `.env` only when overriding defaults:

```env
POSTGRES_PASSWORD=replace-with-a-local-password
FLIGHT_DATABASE_URL=postgres://flight:replace-with-a-local-password@postgres:5432/flight?sslmode=disable
FLIGHT_AUTH_BOOTSTRAP_API_KEY=replace-with-a-secret-at-least-32-characters
```

For native `go run`, change the database host from `postgres` to `localhost`.

See [the operations runbook](docs/OPERATIONS.md) for backup, restore, upgrade, rollback, and release gates.

## How to Run

Flight provides a `Makefile` with shortcuts to run the project effortlessly. You can choose to run it entirely in Docker or natively with Go.

### Option 1: Docker Compose (Recommended)
This is the fastest way to get everything running, including the database and migrations.

```bash
make dev
```
> **What this does:**
> 1. Starts a PostgreSQL database (`docker compose up --build -d postgres`).
> 2. Runs the database migrations (`docker compose run --rm migrate`).
> 3. Starts the Flight server on port `8080` (`docker compose up -d flight`).

To verify it's running, open: `http://localhost:8080/health/ready`

### Option 2: Native Go
If you prefer running the Go code locally for easier debugging:

1. **Start the database:**
```bash
docker compose up -d postgres
```

2. **Run migrations:**
```bash
make migrate
```

3. **Start the server:**
```bash
make run
```
> This will execute `go run ./cmd/flight serve`.

> **Note**: For detailed setup instructions, deployment guides, and API documentation, please refer to the `docs/` folder.

## License

Flight is licensed under the Apache License 2.0.

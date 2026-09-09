# TracePilot

TracePilot is a self-hosted OpenTelemetry trace and log service with an optional
best-effort metrics path. It receives OTLP data, redacts configured sensitive
attributes, applies bounded batch persistence, and stores canonical events plus
first-class spans/logs in PostgreSQL.

With a bootstrap API key configured, it provides project-scoped trace/log
search, service and incident views. Optional replay is disabled by default;
it forwards only redacted JSON bodies and an allowlisted set of non-secret
headers to explicit allowlisted hosts, then records every attempt. A file log
agent is included for shipping local log files.

For package boundaries and runtime flows, see the
[`architecture guide`](docs/ARCHITECTURE.md). For reproducible development,
test, benchmark and Docker commands, see the
[`development/runbook`](docs/DEVELOPMENT.md).
Production drills are in the [`operations runbook`](docs/OPERATIONS.md), and
remaining refactor/product work is tracked in the
[`roadmap`](docs/ROADMAP.md).

## Architecture Overview

```mermaid
flowchart LR
    App[Application / Collector] --> OTLP[OTLP gRPC or HTTP]
    File[Local log file] --> Agent[Agent + disk spool]
    Agent --> OTLP
    Device[Firewall / Syslog sender] -->|TLS + optional mTLS| Syslog[Syslog receiver]
    OTLP --> Auth[API-key scope + project check]
    Auth --> Normalize[Validate, normalize, redact]
    Normalize -->|traces and logs| Durable[Bounded durable pipeline]
    Syslog --> Security[Preserve raw log + security parse]
    Security --> Durable
    Durable -->|single transaction; ACK after commit| PG[(PostgreSQL)]
    Normalize -->|metrics| BestEffort[Best-effort metrics pipeline]
    BestEffort --> PG
    PG --> Detection[Rule detection + incidents]
    Detection -->|same transaction| Outbox[Webhook outbox]
    Outbox --> Worker[SSRF-safe signed delivery worker]
    PG --> API[Scoped management/query API]
    API --> UI[Embedded web UI]
```

Trace and log ingestion is acknowledged only after the PostgreSQL transaction
commits. Metrics intentionally use a separate best-effort queue. Direct Syslog
has no application-level acknowledgement, so senders that need lossless
delivery should use their own disk queue or the included file agent.

## Repository Structure

```text
backend/cmd/                         server and file-agent binaries
backend/internal/domain/             framework-free telemetry/security models
backend/internal/ingest/              OTLP, Syslog and bounded persistence pipeline
backend/internal/storage/postgres/    PostgreSQL adapter and generated sqlc code
backend/internal/transport/httpapi/   management API, auth, OpenAPI and UI wiring
backend/internal/agent/               file tailer, checkpoint and disk spool
backend/db/{migrations,queries}/      schema history and sqlc query source
deploy/                               dev/prod Compose and env examples
docs/                                 architecture, contract and development runbook
scripts/                              backup, restore, load, soak and release drills
```

Package ownership and dependency direction are enforced by
`backend/internal/architecture` tests; the complete map is in
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Configuration

TracePilot uses compiled defaults, then an optional versioned YAML file, then
environment overrides. Copy `deploy/.env.example` to `deploy/.env` when
overriding defaults:

```env
POSTGRES_PASSWORD=tracepilot-dev-only
TRACEPILOT_DATABASE_URL=postgres://tracepilot:tracepilot-dev-only@postgres:5432/tracepilot?sslmode=disable
# TRACEPILOT_AUTH_BOOTSTRAP_API_KEY=replace-with-a-secret-at-least-32-characters
```

For native commands, change the database host from `postgres` to `localhost`.

See the [roadmap](docs/ROADMAP.md) for prioritized refactor, product and
production gates.

## How to Run

The commands below are the direct, cross-platform form. The repository
`Makefile` provides equivalent shortcuts for teams that use GNU Make.

### Option 1: Docker Compose (Recommended)
This starts PostgreSQL, applies migrations and then starts TracePilot:

```powershell
docker compose -f deploy/compose.dev.yaml up --build -d postgres
docker compose -f deploy/compose.dev.yaml run --build --rm migrate
docker compose -f deploy/compose.dev.yaml up --build -d tracepilot
Invoke-WebRequest http://localhost:8080/health/ready
```

The development stack publishes management HTTP on `8080`, OTLP gRPC on
`4317`, OTLP HTTP on `4318`, Syslog on `6514`, and PostgreSQL on `5432`.

### Option 2: Native Go
For local debugging, keep PostgreSQL in Compose and run the Go processes on the
host:

```powershell
docker compose -f deploy/compose.dev.yaml up -d postgres
$env:TRACEPILOT_DATABASE_URL = "postgres://tracepilot:tracepilot-dev-only@localhost:5432/tracepilot?sslmode=disable"
go -C backend run ./cmd/tracepilot migrate up
go -C backend run ./cmd/tracepilot mockdata  # optional deterministic seed
go -C backend run ./cmd/tracepilot serve
```

The server requires the database schema to be at the current migration version.
Full agent, test, SQLC and troubleshooting commands are in
[`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md).

### Console development

The embedded console is a React application in `web/`; its generated static
output remains committed under `backend/internal/webui/dist` so `go build` and
the existing Docker context stay self-contained. Run the API on `:8080`, then:

```powershell
npm --prefix web ci
npm --prefix web run dev
```

The Vite dev server proxies `/api` to TracePilot. Before committing UI changes,
run `npm --prefix web run check`, `npm --prefix web run test`, and
`npm --prefix web run build`; `make web-ci` runs the same gates and verifies
the embedded assets are current.

For the repeatable ingestion baseline, run `make bench-pipeline`; the
PostgreSQL benchmark requires a migrated disposable database and is available
as `make bench-postgres`.

### Main endpoints

| Address | Endpoint | Purpose |
| --- | --- | --- |
| `:8080` | `/health/live`, `/health/ready` | liveness/readiness |
| `:8080` | `/metrics`, `/openapi.json`, `/` | metrics, generated contract, UI |
| `:8080` | `/api/v1/...` | authenticated query and management API |
| `:4317` | OTLP gRPC | trace/log ingestion and best-effort metrics |
| `:4318` | `POST /v1/traces`, `POST /v1/logs`, `POST /v1/metrics` | OTLP HTTP ingestion (`metrics` best-effort) |
| configured | Syslog TCP/TLS | optional security event ingestion |

Webhook management uses `/api/v1/webhooks` with the `admin` scope. Enable the
delivery worker with `TRACEPILOT_NOTIFICATIONS_ENABLED=true` and provide a
32-byte hex or base64 `TRACEPILOT_WEBHOOK_ENCRYPTION_KEY`; create and rotate
operations return the signing secret only once.

### Production Compose

Copy `deploy/.env.production.example` outside the repository, replace every
placeholder with secret-manager or release values, then migrate and start the
immutable image:

```bash
docker compose --env-file /secure/tracepilot.env -f deploy/compose.prod.yaml run --rm tracepilot migrate up
docker compose --env-file /secure/tracepilot.env -f deploy/compose.prod.yaml up -d
```

The TLS directory must contain `tls.crt`, `tls.key`, and the issuing CA as
`ca.crt`; the healthcheck verifies the configured certificate name. When direct
Syslog is enabled in production, the same CA verifies sender client
certificates (`TRACEPILOT_SECURITY_SYSLOG_CLIENT_CA_FILE`).

Production evidence helpers are dependency-light PowerShell scripts:
`scripts/load-test.ps1` supports fixed-request and timed soak modes, reports
p50/p95/p99 and enforces SLO thresholds,
`scripts/query-plan.ps1` captures `EXPLAIN (ANALYZE, BUFFERS)`,
`scripts/upgrade-rollback-drill.ps1` exercises `up → down 1 → up` on a
disposable database, and `scripts/cert-check.ps1` checks PEM expiry, SAN and
the issuer chain. The release workflow builds a multi-architecture image with
BuildKit SBOM/provenance, signs its immutable digest and runs an entrypoint
smoke test.

## License

TracePilot is licensed under the Apache License 2.0.

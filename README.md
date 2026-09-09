# TracePilot

**TracePilot is a self-hosted OpenTelemetry trace, log, and security-event
workspace for small teams that need useful investigation data without sending
telemetry to a third party.** It accepts OTLP, persists traces and logs in
PostgreSQL, and provides a browser console and scoped API for finding failures,
following correlated logs, and investigating security detections.

> Project status: **pre-release / beta**. Trace and log ingestion are the
> primary workflow. Metrics are intentionally best-effort, and some security
> and webhook capabilities are optional. Read [Production readiness](#production-readiness)
> before using TracePilot with production traffic.

## Why TracePilot

- **Own the data path.** Run one immutable application image with PostgreSQL
  you control; telemetry is not forwarded to a hosted TracePilot service.
- **Durability before acknowledgement.** Trace and log ingestion succeeds only
  after its PostgreSQL transaction commits. Backpressure and storage failures
  are retryable instead of silently acknowledged.
- **Useful in the first ten minutes.** The embedded console guides an operator
  from an API key to a first trace; a repeatable sample dataset is available for
  local evaluation.
- **One investigation surface.** Inspect a trace waterfall, correlated logs,
  incidents, security events, detections, and source inventory from the same
  console.
- **Safe defaults.** API keys are project-scoped, replay is disabled by
  default, production requires TLS and authentication, and webhook delivery is
  signed and protected against SSRF.

## What it does today

| Area | Included |
| --- | --- |
| Ingestion | OTLP traces and logs over gRPC and HTTP; bounded batching, redaction, validation, and backpressure. OTLP metrics use a separate best-effort path. |
| Exploration | Trace search, detail waterfall, correlated logs, service views, incident timelines, URL-backed filters, pagination, loading/error/retry states. |
| Security | Optional Syslog TCP/TLS input; Syslog, CEF, and LEEF normalization; rule-driven detections, incidents, source inventory, and raw evidence. |
| Delivery | Optional HTTPS webhooks with encrypted secrets, HMAC signatures, durable outbox retries, and SSRF controls. |
| Operations | Readiness/liveness endpoints, Prometheus metrics, migrations, retention, graceful shutdown, Docker Compose, and operational drill scripts. |
| Agents | A file-log agent with checkpointing and an optional bounded disk spool. |

## Quickstart: local Docker Compose

### Prerequisites

- Docker Engine or Docker Desktop with Docker Compose v2
- A free local port for `8080`, `4317`, `4318`, `5432`, and optionally `6514`

From the repository root, start the full development stack:

```powershell
docker compose -f deploy/compose.dev.yaml up --build -d
Invoke-WebRequest http://localhost:8080/health/ready
```

The stack starts PostgreSQL, runs migrations, then starts TracePilot. If the
readiness request is made before startup completes, repeat it after a few
seconds.

Open [http://localhost:8080](http://localhost:8080) and connect the console
with the development-only key:

```text
tracepilot-development-bootstrap-key-000000
```

To load a deterministic walkthrough containing a failed checkout trace and its
correlated log:

```powershell
docker compose -f deploy/compose.dev.yaml run --build --rm tracepilot mockdata
```

Then open **Trace explorer**, inspect trace
`33333333333333333333333333333333`, and view the correlated log. The fixture is
safe to run more than once and is disabled when `TRACEPILOT_PRODUCTION=true`.

Stop the development stack without deleting data:

```powershell
docker compose -f deploy/compose.dev.yaml down
```

To destroy the local PostgreSQL volume as well:

```powershell
docker compose -f deploy/compose.dev.yaml down --volumes
```

> This last command permanently removes the local development database.

## Send your first application trace

Configure one OpenTelemetry SDK with an **ingest-scoped** API key. The console
shows the same template after connection. For a local HTTP/protobuf exporter:

```powershell
$env:OTEL_SERVICE_NAME = "checkout"
$env:OTEL_EXPORTER_OTLP_PROTOCOL = "http/protobuf"
$env:OTEL_EXPORTER_OTLP_ENDPOINT = "http://localhost:4318"
$env:OTEL_EXPORTER_OTLP_HEADERS = "x-api-key=<your-ingest-api-key>"
```

Use an API key created for the application, rather than sharing an operator or
bootstrap key. The development bootstrap key is intentionally public and must
never be used outside a local machine.

After your instrumented application receives traffic:

1. Open **Trace explorer** in the Console.
2. Filter by service, status, or trace ID; filters remain in the URL.
3. Select **Inspect** to view the span hierarchy and correlated logs.

Trace and log requests are acknowledged only after persistence. Applications
should therefore retain the OpenTelemetry SDK's normal retry behavior for
temporary `429`/`5xx` responses.

## Architecture at a glance

```mermaid
flowchart LR
    App[Application / OTel Collector] --> OTLP[OTLP gRPC or HTTP]
    Agent[File-log agent] --> OTLP
    Device[Syslog sender] --> Syslog[Optional Syslog TCP/TLS]
    OTLP --> Normalize[Authenticate, validate, redact, normalize]
    Syslog --> Normalize
    Normalize --> Durable[Bounded trace/log pipeline]
    Durable --> PG[(PostgreSQL)]
    Normalize --> Metrics[Best-effort metrics pipeline]
    Metrics --> PG
    PG --> API[Scoped API + embedded Console]
    PG --> Detect[Security detection]
    Detect --> Outbox[Optional signed webhook outbox]
```

The key runtime contracts are:

- Trace and log records are committed atomically before OTLP acknowledgement.
- Metrics may be dropped under pressure; they are not on the durability path.
- Direct Syslog has no application-level acknowledgement. Use the included
  agent or a sender-side disk queue when losslessness matters.
- Query and mutation APIs apply the project identity from the API key.

See the [architecture guide](docs/ARCHITECTURE.md) for package ownership,
dependency direction, API contracts, migrations, and detailed runtime flows.

## Interfaces and ports

| Address | Interface | Purpose |
| --- | --- | --- |
| `:8080` | `GET /`, `/openapi.json` | Embedded Console and generated OpenAPI contract |
| `:8080` | `GET /health/live`, `/health/ready` | Liveness and readiness probes |
| `:8080` | `GET /metrics` | Prometheus metrics; authentication is required in production |
| `:8080` | `/api/v1/...` | Authenticated query and management API |
| `:4317` | OTLP gRPC | Trace, log, and best-effort metric ingestion |
| `:4318` | `POST /v1/traces`, `/v1/logs`, `/v1/metrics` | OTLP HTTP ingestion |
| configurable | Syslog TCP/TLS | Optional security-event ingestion |

The live contract is always available from the running server at
[`/openapi.json`](http://localhost:8080/openapi.json). Route registration and
the generated specification use the same manifest, so disabled optional
capabilities are not advertised as available.

## Configuration

Configuration is applied in this order:

1. Compiled defaults
2. Optional versioned YAML configuration file
3. Environment-variable overrides

Copy [`deploy/.env.example`](deploy/.env.example) to `deploy/.env` only when
you need to override development defaults. For native execution, use
`localhost` instead of the Compose hostname `postgres` in the database URL.

Important variables:

| Variable | Purpose |
| --- | --- |
| `TRACEPILOT_DATABASE_URL` | PostgreSQL connection string |
| `TRACEPILOT_AUTH_BOOTSTRAP_API_KEY` | Initial all-scope administrative key; required in production and at least 32 characters |
| `TRACEPILOT_HTTP_ALLOWED_ORIGINS` | Explicit browser origins allowed to call the management API |
| `TRACEPILOT_SECURITY_ENABLED` | Enables the Syslog security-input path |
| `TRACEPILOT_NOTIFICATIONS_ENABLED` | Enables webhook delivery; also requires `TRACEPILOT_WEBHOOK_ENCRYPTION_KEY` |
| `TRACEPILOT_PRODUCTION` | Enables production validation such as TLS and authentication requirements |

Run `tracepilot serve -config path/to/config.yaml` or set
`TRACEPILOT_CONFIG_FILE` to supply YAML configuration. The complete defaults
and validation rules live in [`backend/internal/config`](backend/internal/config).

## Native development

Use native binaries when debugging Go while keeping PostgreSQL in Compose:

```powershell
docker compose -f deploy/compose.dev.yaml up -d postgres
$env:TRACEPILOT_DATABASE_URL = "postgres://tracepilot:tracepilot-dev-only@localhost:5432/tracepilot?sslmode=disable"
$env:TRACEPILOT_AUTH_BOOTSTRAP_API_KEY = "local-development-key-at-least-32-characters"
go -C backend run ./cmd/tracepilot migrate up
go -C backend run ./cmd/tracepilot serve
```

Useful commands:

```powershell
go -C backend run ./cmd/tracepilot version
go -C backend run ./cmd/tracepilot healthcheck
go -C backend run ./cmd/tracepilot migrate version
go -C backend run ./cmd/tracepilot mockdata
```

For console development, start the API on `:8080`, then run:

```powershell
npm --prefix web ci
npm --prefix web run dev
```

The Vite server proxies `/api` and `/health` to TracePilot. Its built assets are
committed to `backend/internal/webui/dist` so the Go binary and Docker image are
self-contained.

## File-log agent

The optional agent tails a newline-delimited file, exports logs over OTLP gRPC,
persists a checkpoint, and spills to a bounded disk buffer when the server is
unavailable.

```powershell
go -C backend run ./cmd/tracepilot-agent `
  -source .\app.log `
  -checkpoint .\var\agent.checkpoint `
  -endpoint localhost:4317 `
  -disk-buffer .\var\agent-spool `
  -api-key-file .\secrets\otlp.key
```

Prefer `-api-key-file` or `TRACEPILOT_AGENT_API_KEY` over putting a secret in
the process command line. See the [development runbook](docs/DEVELOPMENT.md)
for all agent flags and troubleshooting.

## Production readiness

TracePilot's first supported production topology is one immutable application
image, a private managed PostgreSQL instance, and a TLS reverse proxy. The
production Compose file deliberately does **not** start or expose PostgreSQL.

1. Copy [`deploy/.env.production.example`](deploy/.env.production.example) to
   a secret-manager controlled file outside this repository.
2. Set an immutable image digest, database URL, unique bootstrap key, TLS
   directory, certificate server name, and trusted-network port bindings.
3. Run migration as a one-shot operation, then start the image:

   ```bash
   docker compose --env-file /secure/tracepilot.env -f deploy/compose.prod.yaml run --rm tracepilot migrate up
   docker compose --env-file /secure/tracepilot.env -f deploy/compose.prod.yaml up -d
   ```

4. Require `/health/ready` before sending traffic, and verify the schema
   version, metrics, and a scoped API query.

Production containers run non-root, read-only, with a temporary writable
directory and CPU/memory limits. TLS is mandatory; direct Syslog additionally
requires TLS and client-CA verification. Keep OTLP and Syslog listeners on a
trusted network rather than publishing them to the internet.

Before onboarding a design partner, complete the backup/restore, migration,
certificate, load, and outage drills in the [operations runbook](docs/OPERATIONS.md).

## Security model

- API keys carry project identity and scopes. Use least-privilege keys for
  console reads, ingestion, administration, and replay.
- The embedded Console retains its entered key in `sessionStorage` only; it is
  cleared on disconnect or when the browser session ends.
- CORS requires explicit allowed origins. Production management endpoints and
  OTLP listeners use the configured TLS certificate.
- Replay is off by default and requires allowlisted hosts. It forwards only
  redacted JSON and explicitly allowed non-secret headers.
- Webhooks require HTTPS, encrypt stored secrets, sign deliveries with HMAC,
  do not follow redirects, and reject private or metadata-network targets.

Please do not report a suspected credential, cross-project isolation, replay
SSRF, or arbitrary-file-access issue in a public ticket. Read
[SECURITY.md](SECURITY.md) for the reporting policy.

## Development and quality gates

The repository validates backend, console, generated assets, SQLC output, and
container behavior in CI. Run the relevant checks before opening a pull request:

```powershell
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./...
npm --prefix web run check
npm --prefix web run test
npm --prefix web run build
```

GNU Make shortcuts are available for teams using Make:

```bash
make dev          # start local Compose stack
make demo         # start it and seed the walkthrough fixture
make web-ci       # install, type-check, test, build, verify embedded assets
make ci           # full local gate, including SQLC and Go tooling
```

The browser smoke test covers the primary Console path: connect with a key,
find the seeded failed trace, and open its correlated log. Database integration
tests and release checks run in CI. See [CONTRIBUTING.md](CONTRIBUTING.md) and
the [development runbook](docs/DEVELOPMENT.md) for the complete workflow.

## Repository map

```text
backend/cmd/                         TracePilot and file-agent binaries
backend/db/                          PostgreSQL migrations and SQLC source
backend/internal/domain/             Framework-free domain models
backend/internal/ingest/             OTLP, Syslog, bounded pipelines
backend/internal/storage/postgres/   PostgreSQL adapter and generated SQLC code
backend/internal/transport/httpapi/  API, OpenAPI, middleware, embedded UI wiring
backend/internal/webui/              Built Console assets embedded by Go
web/                                 React/Vite Console and browser smoke test
deploy/                              Development and production Compose files
docs/                                Architecture, development, operations, roadmap
scripts/                             Load, query-plan, certificate, rollback drills
```

## Roadmap and contribution

The beta promise is deliberately narrow:

```text
Install → send an OTLP trace/log → find a failed trace → inspect its correlated log
```

Prioritized product and production work is tracked in [docs/ROADMAP.md](docs/ROADMAP.md).
Before making a large change, open an issue to discuss it; small focused fixes
can go directly to a pull request. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

TracePilot is distributed under the [Apache License 2.0](LICENSE).

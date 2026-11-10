# TracePilot

[![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![React](https://img.shields.io/badge/React-20232A?logo=react&logoColor=61DAFB)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![Vite](https://img.shields.io/badge/Vite-646CFF?logo=vite&logoColor=white)](https://vite.dev/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-06B6D4?logo=tailwindcss&logoColor=white)](https://tailwindcss.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-FFCC01?logo=clickhouse&logoColor=black)](https://clickhouse.com/)
[![Redis](https://img.shields.io/badge/Redis-DC382D?logo=redis&logoColor=white)](https://redis.io/)
[![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-F5A800?logo=opentelemetry&logoColor=black)](https://opentelemetry.io/)
[![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)

TracePilot is a self-hosted observability platform for OpenTelemetry traces, logs, and metrics. It provides trace exploration, log correlation, service topology, incident detection, and root cause analysis through a React console and a Go backend.

**Status: Beta.** PostgreSQL stores traces, metrics, incidents, and management data. ClickHouse is required for raw logs. Redis is optional for caching.

## Requirements

- Go matching [backend/go.mod](backend/go.mod).
- Node.js 24 and npm, matching [frontend/.nvmrc](frontend/.nvmrc).
- An existing PostgreSQL database and a reachable ClickHouse service.

Databases run separately; this repository does not include Docker Compose.

## Quick start

Run these commands from the repository root in PowerShell.

### Backend

Copy the configuration template if you do not already have a local file:

```powershell
if (-not (Test-Path backend/.env)) { Copy-Item backend/.env.example backend/.env }
```

Edit `backend/.env`: set `TRACEPILOT_DATABASE_URL`, `TRACEPILOT_CLICKHOUSE_URL`, and the database credentials for your local services. All backend settings belong in this file; process environment variables take precedence.

Each backend shares one PostgreSQL pool (2–20 connections, 5s connect timeout) and one ClickHouse HTTP client (up to 8 connections). Startup validation has a shared deadline (10s by default); readiness probes run concurrently under a shared 2s deadline. Tune pool limits for your workload and total replica count in [backend/.env.example](backend/.env.example).

Initialize both schemas, optionally create a development account, and start the server:

```powershell
go -C backend run ./cmd/tracepilot migrate up
go -C backend run ./cmd/tracepilot migrate clickhouse up
go -C backend run ./cmd/tracepilot seed  # Optional, development only
go -C backend run ./cmd/tracepilot serve
```

The seed account is `admin@tracepilot.local` with password `admin-password-123`. It owns the default project and has super admin access. Seeding is disabled in production and does not generate sample telemetry.

Migrations are explicit; the server does not apply them at startup. When moving existing PostgreSQL logs to ClickHouse, stop ingestion and drain the pipeline, then run `go -C backend run ./cmd/tracepilot migrate clickhouse backfill` after both schemas are ready, before restarting ingestion.

### Console

In another terminal:

```powershell
npm --prefix frontend ci
npm --prefix frontend run dev
```

Open [localhost:5173](http://localhost:5173). Vite proxies `/api` and `/health` to the backend at `localhost:8080`. Sign in with the seeded account. When registration is enabled, new users can register ordinary accounts and create their own projects.

Frontend settings are documented in [frontend/.env.example](frontend/.env.example). When the API uses a different origin, set `VITE_API_BASE_URL` at build time, allow the console origin through `TRACEPILOT_HTTP_ALLOWED_ORIGINS` on the backend, and allow the API origin in your hosting server's Content Security Policy.

## Send telemetry

1. Select or create a project in the console and choose an environment.
2. Open **Settings > Ingest tokens** and create a token for your application.
3. Configure your application's OpenTelemetry SDK exporter, then run the application:

```powershell
$env:OTEL_SERVICE_NAME = "my-service"
$env:OTEL_EXPORTER_OTLP_ENDPOINT = "http://localhost:4318"
$env:OTEL_EXPORTER_OTLP_PROTOCOL = "http/protobuf"
$env:OTEL_EXPORTER_OTLP_HEADERS = "authorization=Bearer%20<ingest-token>,x-environment=development"
```

Replace `<ingest-token>` and `development` with your token and environment name. Use an ingest token for telemetry; console access tokens authenticate management API requests.

| Interface | Default address | Purpose |
| --- | --- | --- |
| Console | `http://localhost:5173` | Development UI |
| Management API | `http://localhost:8080/api/v1` | Authenticated REST API |
| OTLP HTTP | `http://localhost:4318` | `/v1/traces`, `/v1/logs`, `/v1/metrics`; Protobuf or JSON |
| OTLP gRPC | `localhost:4317` | Traces, logs, and metrics |

Health checks are available at `/health/live` and `/health/ready` on port `8080`. Readiness checks both databases.

## Docker

Build the backend and console images independently:

```powershell
docker build -t tracepilot ./backend
docker build -t tracepilot-console ./frontend
```

Use database hostnames reachable from the backend container in `backend/.env`, then initialize the schemas and start the backend:

```powershell
docker run --rm --env-file backend/.env tracepilot migrate up
docker run --rm --env-file backend/.env tracepilot migrate clickhouse up
docker run --rm --env-file backend/.env tracepilot seed  # Optional, development only
docker run -d --name tracepilot --env-file backend/.env -p 8080:8080 -p 4317:4317 -p 4318:4318 tracepilot
```

Start the console with a backend address reachable from its container:

```powershell
docker run -d --name tracepilot-console -p 8081:8080 -e "TRACEPILOT_API_UPSTREAM=http://<backend-host>:8080" tracepilot-console
```

Replace `<backend-host>` with a reachable hostname or IP, then open [localhost:8081](http://localhost:8081). The console image serves the UI through nginx and proxies API requests to `TRACEPILOT_API_UPSTREAM`.

## Project structure

```text
backend/
  cmd/tracepilot/          CLI, dependency wiring, process lifecycle
  db/
    migrations/           PostgreSQL schema migrations
    queries/              SQL queries used by sqlc
  internal/
    application/          Use cases and background workflows
    domain/               Pure models and business rules
    ingest/               OTLP receivers, normalization, bounded queues
    transport/httpapi/    REST handlers, authentication boundary, responses
    storage/              PostgreSQL, ClickHouse, migrations, partitions
    config/               Configuration loading and validation
    auth/                 Tokens, passwords, access rules
    cache/                Memory and Redis cache implementations
    query/                Query contracts and read models
    observability/        Internal logging and metrics
frontend/
  src/
    app/                  App shell, routing, authentication
    features/             Feature pages and components
    shared/               Shared API client, types, UI, charts
    styles/               Base styles, themes, self-hosted fonts
docs/                     Flow reference, diagrams, Postman collection
```

The backend runs as one process. Keep business rules in `domain`, workflows in `application`, and protocol or database details in their adapters.

## Development

```powershell
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./...
npm --prefix frontend run check
npm --prefix frontend run test
npm --prefix frontend run build
```

The frontend build is written to `frontend/dist`. After changing PostgreSQL queries, run `make sqlc-check` with Make and Docker installed; do not edit generated sqlc files manually.

Integration tests require disposable databases. Enable PostgreSQL tests with `TRACEPILOT_INTEGRATION_TEST=1` and `TRACEPILOT_DATABASE_URL` pointing to a migrated test database. ClickHouse tests use `TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1` and `TRACEPILOT_CLICKHOUSE_TEST_URL`. These tests are skipped unless enabled; unit tests alone do not establish production readiness.

Keep contributions focused and add regression tests for behavior changes. Never commit credentials or real telemetry. Report vulnerabilities privately through the repository's security reporting channel.

For production, set `TRACEPILOT_PRODUCTION=true`, replace sample secrets with persistent random values, use HTTPS and PostgreSQL `sslmode=verify-full`, and keep replay disabled unless its destination allowlist has been reviewed. See [backend/.env.example](backend/.env.example) for configuration options.

## Reference

- [Application flow](docs/FLOW.md)
- [Architecture diagrams](docs/diagrams.html) (open in a browser)
- [Postman API collection](docs/tracepilot.postman_collection.json)
- [MIT license](LICENSE)

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

Each backend shares one PostgreSQL pool (2–20 connections, 5s connect timeout) and one ClickHouse HTTP client (up to 8 connections). Startup validation has a shared deadline (10s by default); readiness probes run concurrently under a shared 2s deadline. Every database setting, including migration timeouts and the partition window, is listed with its default in [backend/.env.example](backend/.env.example); tune pool limits for your workload and total replica count there. All commands log JSON through `log/slog`.

Initialize each schema independently, optionally create a development account, and start the server:

```powershell
go -C backend run ./cmd/tracepilot migrate postgres up
if ($LASTEXITCODE -ne 0) { throw "PostgreSQL migration failed; server was not started." }
go -C backend run ./cmd/tracepilot migrate clickhouse up
if ($LASTEXITCODE -ne 0) { throw "ClickHouse migration failed; server was not started." }
go -C backend run ./cmd/tracepilot seed  # Optional, development only
if ($LASTEXITCODE -ne 0) { throw "Seed failed; server was not started." }
go -C backend run ./cmd/tracepilot serve
```

Seeding creates only the super admin account `admin@tracepilot.local` with password `admin-password-123`. It does not create projects, environments, memberships, or sample telemetry. Create a project in the console after signing in; server startup does not create a default project. Seeding is disabled in production, and rerunning it preserves existing accounts and data.

Automatic migration defaults to off for each database. To migrate during `serve` (including the Docker default command), set `TRACEPILOT_DATABASE_AUTO_MIGRATE=true` for PostgreSQL and/or `TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE=true` for ClickHouse in `backend/.env`. Each flag upgrades only its own database, before any listener or worker starts; failure stops startup. Logs live only in ClickHouse, with no PostgreSQL fallback. PostgreSQL migration `000002` drops the legacy `logs` table, so an older installation that still keeps logs in PostgreSQL must copy them with the previous release's `migrate clickhouse backfill` before upgrading.

With Make installed, `make start` (or `make dev`) runs `migrate all` and starts the backend only after migration succeeds, including with parallel Make enabled. The release/update step should use the same binary or image for migration and serving, and gate startup on the migration exit code.

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
docker run --rm --env-file backend/.env tracepilot migrate postgres up
if ($LASTEXITCODE -ne 0) { throw "PostgreSQL migration failed; server was not started." }
docker run --rm --env-file backend/.env tracepilot migrate clickhouse up
if ($LASTEXITCODE -ne 0) { throw "ClickHouse migration failed; server was not started." }
docker run --rm --env-file backend/.env tracepilot seed  # Optional, development only
if ($LASTEXITCODE -ne 0) { throw "Seed failed; server was not started." }
docker run -d --name tracepilot --env-file backend/.env -p 8080:8080 -p 4317:4317 -p 4318:4318 tracepilot
```

Start the console with a backend address reachable from its container:

```powershell
docker run -d --name tracepilot-console -p 8081:8080 -e "TRACEPILOT_API_UPSTREAM=http://<backend-host>:8080" tracepilot-console
```

Replace `<backend-host>` with a reachable hostname or IP, then open [localhost:8081](http://localhost:8081). The console image serves the UI through nginx and proxies API requests to `TRACEPILOT_API_UPSTREAM`.

## Production bundle

Package the current working tree without committing, publishing or connecting to a database:

~~~powershell
powershell -ExecutionPolicy Bypass -File scripts/package-release.ps1
~~~

The default target is Linux amd64; use -Platform linux/arm64 for an ARM server with Docker emulation on the build host. Output is under bin/releases: a tar.gz archive and its .sha256 file. The bundle contains both Docker images, their IDs and build metadata in release.json, input hashes, SHA256SUMS, this README, the existing flow/API docs, nginx.conf.example and the same backend/frontend configuration templates. Actual .env files, certificates, passwords and telemetry are excluded. An uncommitted build is marked dirty; its input hashes identify the working-tree content. The generated backend template enables production, closes registration, requires TLS databases and distinct secrets, and keeps automatic migration disabled.

On a Linux host with Docker and an HTTPS reverse proxy, copy the archive and checksum file, then:

~~~bash
set -euo pipefail
sha256sum -c tracepilot-<version>-linux-amd64.tar.gz.sha256
tar -xzf tracepilot-<version>-linux-amd64.tar.gz
cd tracepilot-<version>-linux-amd64
sha256sum -c SHA256SUMS
docker load -i images.tar
BACKEND_IMAGE="$(sed -n '1p' images.txt)"
CONSOLE_IMAGE="$(sed -n '2p' images.txt)"
docker run --rm "$BACKEND_IMAGE" version
umask 077
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
mkdir -p certs
chmod 755 certs
~~~

Edit backend/.env on the server: replace every changeme value, database hostname/credential and console origin. Generate three independent persistent secrets with openssl rand -hex 32 for access JWT, ingest JWT and metrics. URL-encode PostgreSQL password characters. Install the PostgreSQL public CA at certs/postgres-ca.pem and set its mode to 644 so container UID 65532 can read it. For a private ClickHouse CA, supply a public CA bundle with the same permissions and set SSL_CERT_FILE=/etc/tracepilot/ca-bundle.pem in the same backend/.env. Do not put server TLS private keys in this bundle. Leave registration closed and replay disabled. The template uses 8 ingestion workers and a 20-connection pool; see **Measured capacity** for the 4 vCPU PostgreSQL sizing result.

The following is the **fresh database** installation path. Existing installations must first back up both databases and rehearse the upgrade on restored copies. Older PostgreSQL migration histories are incompatible with the rebuilt baseline; migration 000002 also drops the old PostgreSQL logs table. Export legacy logs using the previous release before that migration. See **Schema migrations** and the upgrade notes below.

Create an isolated application network; change its subnet and trusted proxy addresses together if this range conflicts with an existing network:

~~~bash
docker network create --subnet 172.30.90.0/24 tracepilot-app
docker run --rm --network tracepilot-app --env-file backend/.env \
  --mount "type=bind,src=$PWD/certs,dst=/etc/tracepilot,readonly" \
  "$BACKEND_IMAGE" migrate all
~~~

Stop if migration fails. Create an operator using a password read from stdin; this works with registration closed, needs only PostgreSQL, records an audit event, and refuses to overwrite an existing email. Never run the development seed command on production.

~~~bash
read -r -s -p 'Administrator password: ' ADMIN_PASSWORD
printf '\n'
printf '%s' "$ADMIN_PASSWORD" | docker run --rm -i --network tracepilot-app \
  --env-file backend/.env \
  --mount "type=bind,src=$PWD/certs,dst=/etc/tracepilot,readonly" \
  "$BACKEND_IMAGE" create-admin --email operator@your-domain.com \
  --name "Administrator" --password-stdin
unset ADMIN_PASSWORD
~~~

Start the two application containers. Database services remain external. The backend and console run as non-root users, with read-only filesystems, bounded logs and no added Linux capabilities. The backend's 60-second stop grace exceeds its shutdown deadline.

~~~bash
docker run -d --name tracepilot --network tracepilot-app --ip 172.30.90.2 \
  --restart unless-stopped --stop-timeout 60 --cpus 2 --memory 1g \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --cap-drop ALL --security-opt no-new-privileges:true \
  --log-opt max-size=10m --log-opt max-file=3 \
  --env-file backend/.env \
  --mount "type=bind,src=$PWD/certs,dst=/etc/tracepilot,readonly" \
  -p 127.0.0.1:8080:8080 -p 127.0.0.1:4318:4318 \
  "$BACKEND_IMAGE"
docker run -d --name tracepilot-console --network tracepilot-app --ip 172.30.90.3 \
  --restart unless-stopped --cpus 1 --memory 128m \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=32m \
  --tmpfs /etc/nginx/conf.d:rw,noexec,nosuid,size=1m,uid=101,gid=101,mode=0700 \
  --cap-drop ALL --security-opt no-new-privileges:true \
  --log-opt max-size=10m --log-opt max-file=3 \
  --env-file frontend/.env -p 127.0.0.1:8081:8080 \
  "$CONSOLE_IMAGE"
docker exec tracepilot healthcheck
docker inspect --format '{{.State.Health.Status}}' tracepilot tracepilot-console
~~~

Install nginx.conf.example into the host nginx configuration after replacing both DNS names and certificate paths. It terminates HTTPS, sends /api directly to the backend with normalized proxy headers and SSE buffering disabled, serves the console, and exposes OTLP/HTTP only through the dedicated HTTPS hostname. Validate with sudo nginx -t before reloading nginx. Only ports 80/443 need public access; do not expose PostgreSQL, ClickHouse, metrics or the loopback container ports publicly. This recipe exposes OTLP/HTTP; OTLP/gRPC needs a separately configured TLS/h2 ingress when required.

Verify HTTPS sign-in/refresh, readiness, an OTLP export using a new ingest token, correlated logs/traces and Live Tail before switching application exporters. Back up both databases regularly and test restoring them into separate instances; a copied Docker image does not back up data.

For an update, retain the previous images and configuration, stop exporters and drain the backend, take database backups, review and apply migrations with the **new** image, then recreate the containers with that image and rerun readiness/smoke checks. Do not remove database volumes or reset migration journals. Roll back only to an application version compatible with the current schema; a destructive or incompatible schema change needs the reviewed database restore procedure, not an automatic down migration.

## Project structure

```text
backend/
  cmd/tracepilot/          CLI, dependency wiring, process lifecycle
  db/
    postgres/migrations/  PostgreSQL SQL files and version sequence
    clickhouse/migrations/ ClickHouse SQL files and independent version sequence
    embed.go              SQL embedded in every backend binary
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

Credential verification and password changes live in `application/session`, behind small storage interfaces. HTTP handlers own request decoding, rate limits, cookies, JWT issuance and error responses. `application/live` owns the bounded, process-local telemetry hub; the ingestion persistence callback publishes to it and the HTTP adapter serializes its events as SSE. Neither application package imports the transport or storage implementations.

Console features read authentication through `shared/session`; they do not import the app shell. `shared/session/model.ts` holds the session types and role helpers, and `context.tsx` exposes the React context. The app composes routes and owns the session lifecycle. Overview explicitly composes onboarding and the performance trend; other features stay independent. Architecture tests enforce these dependencies, including lazy route imports.

## MVP scope and release criteria

The current feature set defines the MVP. The intended user journey is: create a project and environment, issue an ingest token, send telemetry, investigate a failed trace and its logs, then manage the resulting incident. Existing optional features remain available through their configuration flags.

| Capability already implemented | Source ownership | Acceptance for the MVP |
| --- | --- | --- |
| Accounts, refresh sessions, temporary passwords and project roles | `auth`, `application/session`, HTTP identity handlers, console session/settings | Sign in, refresh, change password and sign out; account locks and revoked project access take effect; owners and users keep their distinct permissions. |
| Projects, environments, members, ingest tokens and audit | PostgreSQL identity store, HTTP project/user handlers, settings/onboarding | Complete setup from the console; reads and writes stay inside the authorized project/environment; revoked ingest tokens are rejected. |
| OTLP traces, logs and metrics | `ingest`, `application/logs`, PostgreSQL and ClickHouse stores | Exercise HTTP and gRPC ingestion; invalid records return partial success, overload is bounded, and storage failures never acknowledge an unpersisted batch. |
| Investigation and performance | `query`, explorer/analytics stores, traces/logs/performance/topology features | Follow a trace to its spans and correlated logs; preserve scope and time window; show empty, error, pagination and truncation states accurately. |
| Incidents and root cause evidence | Domain incident/analysis, application incident/detection, operations feature | Group repeated failures, inspect evidence, change incident status and retain history without crossing project boundaries. |
| Live Tail, webhooks and HTTP replay | `application/live`, `notification`, `replay`, their HTTP adapters | Live Tail cannot block ingestion; enabled webhooks retain signed delivery/retry history; replay remains opt-in and obeys destination and concurrency limits. |
| Process and data lifecycle | `cmd/tracepilot`, storage migrations/maintenance, observability | Readiness covers both stores; shutdown drains accepted work; retention runs within its bounds; migrations and restore are rehearsed on disposable data. |

MVP acceptance requires the unit/architecture suites, Go vet/build, frontend typecheck/tests/build, disposable PostgreSQL and ClickHouse integration suites, and the browser smoke journey in CI. A passing unit suite does not certify production readiness. Before a release, also complete these checks:

- Rehearse fresh installation, restart with existing data, failed migration recovery, and backup/restore for both stores. The rebuilt, unreleased PostgreSQL history does not upgrade the previous history; choose a supported starting schema and document any required data transfer before shipping.
- Run a representative telemetry workload and record its shape, sustained events/second, queue occupancy/rejections, query p95, memory, storage growth and drain time. Set the supported deployment envelope from those measurements.
- Exercise the complete browser journey with real disposable stores: setup → ingest → trace/log investigation → incident transition; repeat with a regular member and a second project/environment to verify isolation.

### Measured capacity

First measurement, 2026-11-12, on Docker Desktop (Windows, 8 shared vCPU; the load generator ran on the same host). Each container had fixed limits: backend 2 vCPU / 1 GiB, PostgreSQL 17 2 vCPU / 2 GiB, ClickHouse 26.7 2 vCPU / 2 GiB. Workload: OTLP/HTTP JSON, half spans and half logs, 512 records per request, 8 spans per trace, 2% errors, 10s warm-up then 60s measured, a fresh database for each configuration. Treat these numbers as a lower bound for this shape, not an SLO.

| Setting | Offered records/s | Result |
| --- | --- | --- |
| Defaults (`INGESTION_WORKERS=2`) | 1,000 | 0 failures, p95 ≤ 1s |
| Defaults | 2,000 | ~6% 503 (queue full), p95 ~3s |
| `INGESTION_WORKERS=8` | 1,500 | 0–3% 503 across two runs, p95 1.7–6.4s |
| `INGESTION_WORKERS=8` | 3,000–4,000 | PostgreSQL CPU saturated; 1,100–1,300 records/s accepted |

- Backend memory stayed below 100 MiB; PostgreSQL grew about 1.4 KB per span with indexes, ClickHouse under 100 bytes per log.
- Before requests were admitted as a whole, overload persisted 46–51% of records for requests that still returned 503; goodput at 3,000 records/s rose from 674 to 1,263 records/s after the change.
- In that first measurement, console reads competed with ingestion for PostgreSQL: at 1,000 records/s, 17–36% of reads returned 503. Trace lists still aggregate all matching traces before pagination, so cost grows with the window's span count.
- A trace list without `from` (and without `traceId`) reads only the hour before `to`, its page cursor, or now. The console checks its connection with `GET /api/v1/projects/{id}` instead of listing traces.
- `docker stop` under 1,500 records/s exited cleanly in about 1.2s without failing in-flight requests. With ClickHouse stopped, readiness returned 503 and login kept working; readiness recovered about 30s after ClickHouse returned, without restarting the backend.

Follow-up on 2026-11-16 used the production server image with the same container limits, `INGESTION_WORKERS=8`, `QUEUE_WORKERS=8`, and a new project in an existing disposable database for each run. At 500 spans + 500 logs/s and 5 console reads/s, topology and trace lookups could choose quadratic plans for a new project absent from PostgreSQL statistics. Parameterized lateral lookups removed that failure: read failures fell from 223/300 (74.33%, including six 504s) to 101/300 (33.67%, all 503s); topology p95 fell from 20.502s to 9.812s. PostgreSQL still saturated its 2 vCPU limit, and two of 59 trace exports returned 503. This workload exceeds the measured interactive capacity; the original ingest-oriented 1,000 records/s figure is not a guarantee with concurrent console reads.

Final runs with that same code and 5 reads/s:

| PostgreSQL CPU limit | Offered records/s | Failed ingest requests | Failed reads | Read p95 |
| --- | --- | --- | --- | --- |
| 2 vCPU | 500 | 0/60 | 2/300 (503) | 700ms |
| 2 vCPU | 1,000 | 2/118 (503) | 101/300 (503) | 3.621s |
| 4 vCPU | 1,000 | 0/118 | 0/300 | 689ms |

Only PostgreSQL's CPU limit changed in the last run; its memory stayed at 2 GiB, backend at 2 vCPU / 1 GiB, and ClickHouse at 2 vCPU / 2 GiB. Ingest p95 was 940ms for traces and 942ms for logs; topology p95 was 1.183s, and the query gate peaked at three of four slots. Use 4 vCPU PostgreSQL as a starting point for testing this interactive workload, not as a production guarantee: these 60s runs do not establish long-term capacity for a full one-hour query window or larger retained datasets.

Keep `DATABASE_MAX_CONNECTIONS` ≥ 2 × ingestion workers + 2. Repeat the measurement with `backend/cmd/loadgen` after changing the workload shape, query window, data volume or hardware:

```powershell
go -C backend run ./cmd/loadgen -spans-per-second 500 -logs-per-second 500 -spans-per-request 512 -logs-per-request 512 -reads-per-second 5 -duration 60s
```

The tool signs in (`-email`, `-password`), creates its own project, environment and ingest token, and prints latency percentiles, failures by status, per-endpoint read latency and the server's queue and pool readings from `/metrics` (`-metrics-token` in production).

### Backup, restore and migration recovery

PostgreSQL and ClickHouse are backed up separately, so a pair taken while ingesting can differ by the records written in between. For a consistent pair, stop ingestion and let the pipeline drain first; otherwise restore both from the same window and let exporters resend. Rehearsed on 2026-11-12 with 264 MB PostgreSQL and 180k logs: `pg_dump` took 12s (22 MB), `pg_restore -j 2` 25s, ClickHouse backup 1s and restore 2s; a backend on the restored pair passed readiness and served traces and logs.

```bash
# PostgreSQL
pg_dump -Fc -d "$TRACEPILOT_DATABASE_URL" -f tracepilot.dump
createdb tracepilot_restore && pg_restore --no-owner -j 2 -d tracepilot_restore tracepilot.dump
# ClickHouse: the server needs <backups><allowed_path>/backups/</allowed_path></backups> and a directory owned by the clickhouse user
clickhouse-client --query "BACKUP DATABASE tracepilot TO File('/backups/tracepilot-2026-11-12')"
clickhouse-client --query "RESTORE DATABASE tracepilot AS tracepilot_restore FROM File('/backups/tracepilot-2026-11-12')"
```

Point `TRACEPILOT_DATABASE_URL` and `TRACEPILOT_CLICKHOUSE_DATABASE` at the restored pair and check `migrate postgres status`, `migrate clickhouse status` and `/health/ready` before routing traffic.

A failed migration can leave a journal dirty. Then `migrate ... status` reports `dirty: true`, `migrate up` refuses, and `serve` exits before opening listeners. To recover PostgreSQL, compare `pg_dump --schema-only` of the dirty database with a reference database at the same version. If they match, the migration finished and only the journal is stale: `UPDATE schema_migrations SET dirty = false WHERE version = <version>`. If they differ, restore from backup, or reverse the partial change by hand, set the previous version clean and run `migrate up` again. ClickHouse keeps the clean row of a version over its dirty row; after verifying the table, insert a clean row for that version into `tracepilot_schema_migrations` (the PostgreSQL path was rehearsed; this ClickHouse step was not). Never force a version you have not verified.

## Development roadmap

Work in this order. Each stage produces a releasable increment; the exit criteria determine when to move on.

| Priority / stage | Deliverable | Exit criteria and dependency |
| --- | --- | --- |
| P0 — Validate the MVP | Complete the release checks above, resolve failures, document the supported migration baseline and first measured capacity limits. | All required CI jobs pass with both disposable databases; the real browser journey, backup/restore and workload evidence are reviewed. Blocks a production release. |
| P1 — Improve first use and account recovery | Add tested OTel setup examples and token/environment diagnostics; provide an operator-reviewed password recovery flow with expiring, single-use tokens. | A new user reaches a first trace using the published steps; recovery tokens expire, cannot be replayed and do not expose account existence. Depends on the P0 identity/isolation baseline and an agreed email delivery mechanism. |
| P2 — Make incident operations configurable | Introduce project/environment detection rules with an evaluation preview, then notification routing and deduplication controls. | Rules are scoped, validated and audited; deterministic fixtures prove firing/recovery behavior; notification retries preserve the existing outbox guarantees. Depends on stable incident and delivery behavior from P0. |
| P3 — Add capacity where measurements require it | Profile expensive queries and cardinality, set per-project ingestion/query budgets, and optimize the measured bottleneck. Consider durable queues or shared Live Tail fan-out when durability or multi-replica requirements justify them. | Repeat the P0 workload before/after; show the measured gain and retained isolation/backpressure. Extract a process only when a documented requirement or benchmark justifies the operational cost. |

SSO/MFA, destructive project/environment lifecycle, billing and broad dashboard customization are outside this MVP. Revisit them against user demand after P0; they do not block use of the existing feature set.

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

PostgreSQL development follows database-first: add a SQL migration, update the named SQL queries, regenerate sqlc, then adapt the storage code and run integration tests on a disposable database. `backend/sqlc.yaml` reads the same migration history embedded in the binary; `db/external` only describes migration-tool metadata for code generation. Domain and API models remain separate from generated database rows.

The database also enforces incident status/severity and nonempty, one-dimensional webhook subscriptions to supported events. These checks apply to direct SQL writes and imports as well as API writes. Adding a new state or event requires updating both its domain rule and a new migration. Constraint migrations reject existing invalid rows; inspect and repair those rows before retrying, without automatically deleting data or clearing a dirty journal.

Integration tests require disposable databases. Enable PostgreSQL tests with `TRACEPILOT_INTEGRATION_TEST=1` and `TRACEPILOT_DATABASE_URL` pointing to a migrated test database. ClickHouse tests use `TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1` and `TRACEPILOT_CLICKHOUSE_TEST_URL`. These tests are skipped unless enabled; unit tests alone do not establish production readiness.

Migration integration tests create and drop their own PostgreSQL databases; the disposable test role needs `CREATE DATABASE`. CI also tests ClickHouse, reruns with existing data, atomic baseline failure, dirty-state rejection, lock contention, and running the binary outside the source tree.

### Schema migrations

PostgreSQL and ClickHouse have independent migration managers:

| Database | CLI entry point | Embedded SQL history | Version/dirty journal |
| --- | --- | --- | --- |
| PostgreSQL | `migrate postgres up/down/status/version` | `backend/db/postgres/migrations` | PostgreSQL `schema_migrations` |
| ClickHouse | `migrate clickhouse up/status` | `backend/db/clickhouse/migrations` | ClickHouse `tracepilot_schema_migrations` |

Use `make migrate-postgres` or `make migrate-clickhouse` to upgrade just that database. The existing `make migrate` remains a PostgreSQL alias.

`migrate all` is a convenience batch command: it calls the PostgreSQL manager, then the ClickHouse manager, then checks readiness of both schemas. It has no shared version or journal. It stops at the first error and never starts the server or automatically rolls back a completed step. PostgreSQL and ClickHouse do not share a transaction. After resolving a connectivity failure, rerun the command; completed versions are skipped. A `dirty` version requires inspecting and repairing the failed migration before retrying.

Each database manages its own migration files, target version, history, and auto-migration setting. PostgreSQL commands bind and validate only PostgreSQL settings and production TLS policy; ClickHouse commands bind and validate only ClickHouse settings, log retention and production TLS policy. Missing or malformed settings for the other database do not block an individual command. Both keep their configuration in the single `backend/.env` file. `migrate all` requires both. The server always requires both databases to be ready, even if only one auto-migration flag is enabled.

The CLI routes PostgreSQL commands to `cmd/tracepilot/migrate_postgres.go` and ClickHouse commands to `migrate_clickhouse.go`; each validates its own operations before loading configuration. Startup calls each database's runner directly under its own auto-migration flag. The shared CLI helper handles configuration, signals and logging; the shared status helper only computes a read-only view from the selected history. Neither owns migration state or dispatches between engines. The storage adapters cannot import each other; architecture tests enforce this boundary. `migrate all` reuses the runtime connection/readiness path with database credentials only, so it does not require application JWT or webhook secrets.

To migrate both automatically, set these in `backend/.env` and run `go -C backend run ./cmd/tracepilot serve`:

```dotenv
TRACEPILOT_DATABASE_AUTO_MIGRATE=true
TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE=true
```

Both defaults in `.env.example` are `false`; manual `migrate all` and `make start` remain available.

Inspect each database without changing schema or creating migration metadata:

```powershell
go -C backend run ./cmd/tracepilot migrate postgres status
go -C backend run ./cmd/tracepilot migrate clickhouse status
```

Status reports the current version (`0` means uninitialized), embedded target version, dirty state, and pending versions. Individual commands are `migrate postgres up`, `migrate postgres version`, `migrate postgres down STEPS`, and `migrate clickhouse up`. Omitting `postgres` keeps the existing PostgreSQL shorthand. The initial PostgreSQL baseline has no down SQL: rolling it back is rejected before changing the journal or deleting tables. Later PostgreSQL migrations can have down SQL; inspect it and back up before rollback. ClickHouse has no automatic `down` command.

The PostgreSQL runner owns its advisory lock for every `up`/`down` call, including creation of the migration journal on a fresh database; callers do not acquire a second application lock. `status` and `version` stay read-only and do not wait for that mutation lock. Lock waits are limited to 10s, PostgreSQL migration statements to 5m, and ClickHouse requests to `TRACEPILOT_CLICKHOUSE_REQUEST_TIMEOUT`. Cancellation stops PostgreSQL between files; an in-flight PostgreSQL statement can continue until its statement timeout. ClickHouse manages its own version/dirty journal without a PostgreSQL lock; use a single updater for non-idempotent ClickHouse DDL.

PostgreSQL starts with `backend/db/postgres/migrations/000001_init.up.sql`, which creates the initial tables, constraints, indexes, functions and partitions in one transaction. `000002_drop_postgres_logs` removes the legacy `logs` table; its down SQL recreates only an empty table. ClickHouse keeps its own `000001_tracepilot_logs.up.sql`, because it uses a different SQL dialect and journal. Both histories are embedded in the binary; no runtime SQL path setting is needed.

This consolidated baseline is for fresh PostgreSQL databases. It does not upgrade the earlier per-table or grouped histories. For an existing installation, use a new database and a reviewed data transfer or prepare a separate transition migration; do not reset or force the old journal to version 1. Once published or applied to retained data, never edit, renumber or squash that history. The first ClickHouse migration adopts an existing log table only after validating its schema and retention settings, preserving its rows. TTL changes use the ClickHouse migration command or its auto-migration flag; changing an existing partition key needs a separate table/data migration.

ClickHouse validates the existing engine, partition and deduplication key before changing TTL, then verifies column access before marking the final migration clean. A normal migration or rerun performs one schema check and one column probe; it reads the schema again only when a TTL alteration needs verification. Readiness and migration use the same layout and TTL checks.

### Running and extending migrations

For automatic migration while running source, set these in the existing `backend/.env`:

```dotenv
TRACEPILOT_DATABASE_AUTO_MIGRATE=true
TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE=true
```

Then run from the repository root:

```powershell
go -C backend run ./cmd/tracepilot serve
```

Startup applies pending SQL before opening listeners or starting workers. Migration failure stops startup. An already applied version is skipped; this flag does not compare Go models with the live schema or rerun an edited baseline. Leave both flags false to migrate explicitly:

```powershell
go -C backend run ./cmd/tracepilot migrate all
if ($LASTEXITCODE -ne 0) { throw "Migration failed; do not start the server." }
go -C backend run ./cmd/tracepilot serve
```

For a later PostgreSQL schema change, choose the next unused sequence number, currently `000003`, and add `000003_projects_add_description.up.sql` plus its `.down.sql` partner. Put only that change in the new files. Each database numbers its migrations independently; numbers do not restart for a new feature or application release. After editing schema or queries, regenerate and verify sqlc with `make sqlc-check`, adapt storage code, and test on a migrated disposable database. Keep large data backfills in explicit bounded workflows.

### Application versions

Application releases use [Semantic Versioning](https://semver.org/): `MAJOR.MINOR.PATCH`, for example `0.1.0-beta.1`. Before 1.0, use a new minor version for a new feature or breaking change and a patch for a compatible fix. From 1.0, incompatible API changes increment major, compatible features increment minor, and compatible fixes increment patch. Use `-beta.1`, `-beta.2` or `-rc.1` for trial releases; do not replace a published tag.

Keep application versions separate from schema versions. For example, application `v0.1.1` may still use PostgreSQL migration 1; application `v0.2.0` may include PostgreSQL migration 2 while ClickHouse remains at migration 1. Git commit hashes identify individual source revisions.

The release workflow uses Git tags such as `v0.1.0-beta.1`, runs CI, embeds version/commit/build time into the backend, and publishes Linux binaries plus backend and console images. Both images receive SBOM/provenance and keyless signatures. The console image uses the same tag at the repository name with a -console suffix. Local offline bundles use scripts/package-release.ps1 and do not create tags or contact a registry. `frontend/package.json` is private package metadata; when preparing a release, synchronize it and its lockfile with `npm --prefix frontend version 0.1.0-beta.1 --no-git-tag-version` if needed, without creating a second tag.

After reviewing and committing the intended release, these commands publish it through the existing workflow; run them only when ready to release:

```powershell
git tag -a v0.1.0-beta.1 -m "TracePilot 0.1.0-beta.1"
git push origin v0.1.0-beta.1
```

For a local Windows build with version metadata, without creating or publishing a tag:

```powershell
$releaseVersion = '0.1.0-beta.1'
$releaseCommit = (git rev-parse --short HEAD).Trim()
$releaseBuiltAt = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
go -C backend build -trimpath -ldflags "-s -w -X main.version=$releaseVersion -X main.commit=$releaseCommit -X main.buildTime=$releaseBuiltAt" -o ../bin/tracepilot.exe ./cmd/tracepilot
./bin/tracepilot.exe version
```

Plain `go run ./cmd/tracepilot version` from `backend` reports `dev`; release builds receive the Git tag through build flags.

Keep contributions focused and add regression tests for behavior changes. Never commit credentials or real telemetry. Report vulnerabilities privately through the repository's security reporting channel.

For production, set `TRACEPILOT_PRODUCTION=true`, replace sample secrets with persistent random values (including a separate `TRACEPILOT_AUTH_INGEST_JWT_SECRET`, which production requires), use HTTPS and PostgreSQL `sslmode=verify-full`, and keep replay disabled unless its destination allowlist has been reviewed. See [backend/.env.example](backend/.env.example) for configuration options.

When upgrading from access tokens without the `tp_ver` claim, accounts that have previously changed passwords or been disabled renew their token through refresh or sign in again. Log IDs now include the full nanosecond timestamp; an exporter retry spanning this upgrade can store a record under both its old and new ID. Existing rows are preserved; already collapsed logs require resending from the original source.

## Reference

- [Application flow](docs/FLOW.md)
- [Architecture diagrams](docs/diagrams.html) (open in a browser)
- [Postman API collection](docs/tracepilot.postman_collection.json)
- [MIT license](LICENSE)

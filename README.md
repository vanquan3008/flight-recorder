# TracePilot

[![Beta](https://img.shields.io/badge/status-beta-yellow)](#mvp-scope-and-release-criteria)
[![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)](backend/go.mod)
[![React](https://img.shields.io/badge/React-20232A?logo=react&logoColor=61DAFB)](frontend/package.json)
[![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-F5A800?logo=opentelemetry&logoColor=black)](#send-telemetry)
[![MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

TracePilot is a self-hosted observability platform for OpenTelemetry traces, logs, and metrics. Use its React console to follow a failed trace to the logs, see which services are affected, and investigate the incident.

[Quick start](#quick-start) · [Send telemetry](#send-telemetry) · [Deployment](#production-deployment) · [Development](#development) · [Reference](#reference)

## Features

- Explore traces, correlate spans with logs, and follow incoming events in Live Tail.
- View service topology and performance trends, investigate incidents, and review root-cause evidence.
- Manage projects, environments, member roles, ingest tokens, and audit history.
- Send signed webhooks and replay HTTP requests when those features are enabled.

## Requirements

| Dependency | Purpose |
| --- | --- |
| Go, using the version declared in [`backend/go.mod`](backend/go.mod) | Run or build the backend from source |
| Node.js 24 and npm, matching [`frontend/.nvmrc`](frontend/.nvmrc) | Run or build the console from source |
| PostgreSQL | Store traces, metrics, incidents, and management data |
| ClickHouse | Store raw logs |
| Redis (optional) | Distributed cache |

Provision PostgreSQL and ClickHouse separately. For a fresh setup, create the PostgreSQL database before running migrations. Use database credentials that can apply the schemas. The ClickHouse migration can create its configured database.

## Quick start

Run the commands from the repository root. The examples use PowerShell.

### 1. Configure the backend

```powershell
if (-not (Test-Path backend/.env)) {
    Copy-Item backend/.env.example backend/.env
}
```

Set the following values in `backend/.env`. Variables already set in the shell take precedence over the file. See [the backend template](backend/.env.example) for the full list and defaults.

| Setting | What to enter |
| --- | --- |
| `TRACEPILOT_DATABASE_URL` | Connection URL for your PostgreSQL database. URL-encode special characters in the password. |
| `TRACEPILOT_CLICKHOUSE_URL` | ClickHouse HTTP endpoint, for example `http://localhost:8123`. |
| `TRACEPILOT_CLICKHOUSE_DATABASE`, `TRACEPILOT_CLICKHOUSE_USERNAME`, `TRACEPILOT_CLICKHOUSE_PASSWORD` | Your ClickHouse database and credentials. |
| `TRACEPILOT_AUTH_JWT_SECRET` | A persistent random secret of at least 32 characters. Use `openssl rand -hex 32` if OpenSSL is installed, or the PowerShell example below. |

<details>
<summary>Generate a secret with PowerShell</summary>

```powershell
$secretBytes = New-Object byte[] 32
$random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$random.GetBytes($secretBytes)
$random.Dispose()
[Convert]::ToBase64String($secretBytes)
```

</details>

### 2. Migrate and start the backend

```powershell
go -C backend run ./cmd/tracepilot migrate all
if ($LASTEXITCODE -ne 0) { throw 'Database migration failed.' }

go -C backend run ./cmd/tracepilot serve
```

`migrate all` applies the PostgreSQL and ClickHouse schemas and checks both stores. With Make installed, `make start` or `make dev` runs the same two steps. Existing installations should review [schema migrations](#schema-migrations) before upgrading.

### 3. Launch the console

In a second terminal, from the repository root:

```powershell
npm --prefix frontend ci
if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
npm --prefix frontend run dev
```

Open [localhost:5173](http://localhost:5173) and register an account. Local configuration enables registration by default. Vite proxies `/api` and `/health` to the backend on port `8080`.

<details>
<summary>Optional development administrator</summary>

Run `go -C backend run ./cmd/tracepilot seed` after migration. Sign in as `admin@tracepilot.local` with `admin-password-123`. The command creates only this administrator and preserves existing accounts on rerun. Create your project and environment from the console. Seeding is disabled in production.

</details>

<details>
<summary>Run the quick start in Bash</summary>

```bash
test -e backend/.env || cp backend/.env.example backend/.env
```

Edit the settings from step 1, then run:

```bash
go -C backend run ./cmd/tracepilot migrate all &&
  go -C backend run ./cmd/tracepilot serve
```

In a second terminal:

```bash
npm --prefix frontend ci && npm --prefix frontend run dev
```

</details>

## Send telemetry

1. Create a project in the console and select or create an environment.
2. As the project owner, open **Settings → Ingest tokens** and create a token.
3. Set these variables in the terminal that starts your OpenTelemetry-instrumented application. Replace the token and environment with your own values.

```powershell
$env:OTEL_SERVICE_NAME = 'my-service'
$env:OTEL_EXPORTER_OTLP_ENDPOINT = 'http://localhost:4318'
$env:OTEL_EXPORTER_OTLP_PROTOCOL = 'http/protobuf'
$env:OTEL_EXPORTER_OTLP_HEADERS = 'authorization=Bearer%20<ingest-token>,x-environment=development'
```

In Bash, use `export NAME='value'` for the same variables. Generate traffic and open **Traces** in the selected environment. Open a trace to inspect its spans and any logs that carry its trace ID.

<details>
<summary>Generate sample traces locally</summary>

With the backend running and the optional development administrator seeded, run from the repository root:

```powershell
go -C backend run ./cmd/loadgen -spans-per-second 16 -spans-per-request 8 -warmup 0s -duration 5s
```

The command creates a project named `Load <timestamp>` and an environment named `load`. Select them in the console and open **Traces**.

</details>

| Interface | Local default |
| --- | --- |
| Console | [http://localhost:5173](http://localhost:5173) |
| Management API | `http://localhost:8080/api/v1` (console access token) |
| OTLP/HTTP | `http://localhost:4318` (ingest token). Supports Protobuf or JSON on `/v1/traces`, `/v1/logs`, and `/v1/metrics`. |
| OTLP/gRPC | `localhost:4317` (ingest token) |
| Health | [`/health/live`](http://localhost:8080/health/live) checks the process. [`/health/ready`](http://localhost:8080/health/ready) checks both databases. |

If readiness returns `503`, check both database connections and the migration status. If the exporter gets `401`, check that the authorization header contains an active **ingest token**. If an export succeeds and the console is empty, check the selected project, environment, and time range.

## Production deployment

Use Docker Engine 25+ with BuildKit and Docker Compose 2.24+ for the container workflow. [`deploy/compose.yaml`](deploy/compose.yaml) connects to external PostgreSQL and ClickHouse services. It runs `migrate` once, starts the backend after migration succeeds, and starts the console once the backend is healthy.

### Container images

Build from the repository root or use tagged release images.

```powershell
docker build --target server -t tracepilot:local backend
docker build --target console -t tracepilot-console:local frontend
```

The backend runs as UID 65532 in a distroless image. The console runs as UID 101 in nginx and proxies API requests to the backend. Both images accept `VERSION`, `COMMIT`, and `BUILD_TIME` as build arguments. Pass `--platform linux/arm64` to build ARM images. Running them on amd64 requires emulation.

For a same-origin setup, leave the console build argument `VITE_API_BASE_URL` empty and set the runtime upstream `TRACEPILOT_API_UPSTREAM` in `frontend/.env`. For a separate API origin, set `VITE_API_BASE_URL` at build time, allow the console origin in the backend's CORS configuration, and allow the API origin in the console's Content Security Policy. [Frontend configuration template](frontend/.env.example)

### Stack and configuration

Run these Bash commands from the repository or extracted release bundle on the deployment host.

```bash
umask 077
test -e backend/.env || cp backend/.env.example backend/.env
test -e frontend/.env || cp frontend/.env.example frontend/.env
mkdir -p certs
chmod 755 certs
```

| Location | Production configuration |
| --- | --- |
| `backend/.env` | Set database endpoints reachable from the containers. Use PostgreSQL with `sslmode=verify-full` and ClickHouse over HTTPS. |
| `backend/.env` | Generate three separate persistent secrets with `openssl rand -hex 32` for `TRACEPILOT_AUTH_JWT_SECRET`, `TRACEPILOT_AUTH_INGEST_JWT_SECRET`, and `TRACEPILOT_HTTP_METRICS_TOKEN`. |
| `backend/.env` | Set `TRACEPILOT_HTTP_ALLOWED_ORIGINS` to the HTTPS console origin, `TRACEPILOT_HTTP_TRUSTED_PROXIES` to `172.30.90.1/32,172.30.90.3/32` for the supplied network, and `TRACEPILOT_AUTH_ALLOW_REGISTRATION=false`. |
| `frontend/.env` | Set `TRACEPILOT_API_UPSTREAM=http://tracepilot:8080`. |
| `certs/` | Store public CA certificates here. Compose mounts the directory at `/etc/tracepilot`. Use mode `644` for public CA files so the backend can read them. Keep TLS private keys with the host proxy. |

Set the PostgreSQL CA in the connection URL with `sslrootcert=/etc/tracepilot/postgres-ca.pem`. For a private ClickHouse CA, set `SSL_CERT_FILE=/etc/tracepilot/ca-bundle.pem` with the corresponding trust bundle. Use single quotes around values with literal `$` characters in Compose env files.

Compose enables production validation and disables auto-migration during `serve`. The migration service uses the same backend image. Host ports bind to loopback (`8080` for the API, `8081` for the console, and `4318` for OTLP/HTTP). If you change the Compose subnet, update the gateway, static IPs, and trusted proxy CIDRs together. Keep the stop grace period longer than the request shutdown and pipeline drain deadlines.

### Start and verify

The defaults below match the local builds. Export your registry or offline bundle tags first to use those images.

```bash
export TRACEPILOT_BACKEND_IMAGE="${TRACEPILOT_BACKEND_IMAGE:-tracepilot:local}"
export TRACEPILOT_CONSOLE_IMAGE="${TRACEPILOT_CONSOLE_IMAGE:-tracepilot-console:local}"

docker compose -f deploy/compose.yaml config --quiet &&
  docker compose -f deploy/compose.yaml up --wait --wait-timeout 180
docker compose -f deploy/compose.yaml ps -a
```

Verify that `migrate` exited with code 0 and that both application containers are healthy. If startup fails, inspect `docker compose -f deploy/compose.yaml logs migrate tracepilot`.

For a fresh database, create the first operator interactively. This command records an audit event and rejects an existing email.

```bash
read -r -s -p 'Administrator password: ' ADMIN_PASSWORD
printf '\n'
printf '%s' "$ADMIN_PASSWORD" | docker compose -f deploy/compose.yaml run --rm -T migrate \
  create-admin --email operator@your-domain.com --name "Administrator" --password-stdin
unset ADMIN_PASSWORD
```

Configure the host HTTPS proxy with [`deploy/nginx.conf.example`](deploy/nginx.conf.example). Replace the example DNS names and certificate paths, run `sudo nginx -t`, and reload nginx. The example exposes the HTTPS console and OTLP/HTTP on separate hostnames and supports SSE for Live Tail. Public OTLP/gRPC requires a TLS/HTTP2 ingress.

Before switching exporters, verify HTTPS sign-in and session refresh, an OTLP export with a new token, correlated trace and log reads, and Live Tail.

### Updates and recovery

Pause exporters, stop the console and backend to drain accepted work, and back up **both databases**. Rehearse the upgrade on restored copies and keep the previous images and configuration. Review [schema compatibility](#schema-migrations), particularly the consolidated PostgreSQL baseline and migration `000002`.

```bash
docker compose -f deploy/compose.yaml stop tracepilot-console tracepilot
# After backing up both databases, set the two image variables to new immutable tags.
docker compose -f deploy/compose.yaml config --quiet &&
  docker compose -f deploy/compose.yaml up --wait --wait-timeout 180
```

Repeat the startup and telemetry checks before resuming exporters. Roll back the application only to a version compatible with the current schemas. Use the rehearsed database restore procedure for incompatible changes.

<details>
<summary>Database backups and migration recovery</summary>

Take backups of both stores after draining ingestion so they reflect the same point in the workflow. Restore to fresh databases and verify them before routing traffic.

```bash
pg_dump -Fc -d "$TRACEPILOT_DATABASE_URL" -f tracepilot.dump
createdb tracepilot_restore && pg_restore --no-owner -j 2 -d tracepilot_restore tracepilot.dump

clickhouse-client --query "BACKUP DATABASE tracepilot TO File('/backups/tracepilot-backup')"
clickhouse-client --query "RESTORE DATABASE tracepilot AS tracepilot_restore FROM File('/backups/tracepilot-backup')"
```

For the ClickHouse example, allow `/backups/` in the server backup configuration and make the directory writable by the ClickHouse user. Point `TRACEPILOT_DATABASE_URL` and `TRACEPILOT_CLICKHOUSE_DATABASE` at the restored pair, then check both migration statuses and `/health/ready`.

A dirty journal blocks migration and startup. Compare the actual schema with a verified reference at the intended version. Verify whether the DDL completed before repairing the journal, or restore the backups and retry. PostgreSQL uses `schema_migrations`. ClickHouse uses `tracepilot_schema_migrations`; a verified clean row supersedes the dirty row for the same version. The PostgreSQL recovery path was rehearsed, while the ClickHouse journal repair remains unverified.

</details>

### Offline release bundle

<details>
<summary>Package, verify, and load images</summary>

```powershell
powershell -ExecutionPolicy Bypass -File scripts/package-release.ps1 -Version 0.1.0-rc.1
```

The script builds and smoke-tests both images. It writes an archive and SHA-256 file under `bin/releases/`. Linux amd64 is the default. Use `-Platform linux/arm64` for ARM and have emulation available if smoke tests run on amd64. The bundle contains the images, metadata, sample env files, `deploy/compose.yaml`, and docs. Builds from modified sources are marked dirty and include input hashes.

On the target Linux host, replace `<version>` with the archive version.

```bash
set -euo pipefail
sha256sum -c 'tracepilot-<version>-linux-amd64.tar.gz.sha256'
tar -xzf 'tracepilot-<version>-linux-amd64.tar.gz'
cd 'tracepilot-<version>-linux-amd64'
sha256sum -c SHA256SUMS
docker load -i images.tar
export TRACEPILOT_BACKEND_IMAGE="$(sed -n '1p' images.txt)"
export TRACEPILOT_CONSOLE_IMAGE="$(sed -n '2p' images.txt)"
docker run --rm "$TRACEPILOT_BACKEND_IMAGE" version
```

Use these exported tags when you complete [configuration](#stack-and-configuration) and [startup](#start-and-verify).

</details>

## Development

For a local ClickHouse and CH-UI stack, set `CLICKHOUSE_PASSWORD` and `CH_UI_SECRET_KEY`, then run `docker compose -f deploy/docker-compose.yaml up -d`. Keep the ClickHouse credentials in this stack consistent with `backend/.env`.

Run these checks after installing frontend dependencies with `npm --prefix frontend ci`.

```powershell
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./...
npm --prefix frontend audit --audit-level=moderate
npm --prefix frontend run check
npm --prefix frontend run test
npm --prefix frontend run build
```

The frontend build outputs to `frontend/dist`. For browser checks, see the Playwright workflow in [CI](.github/workflows/ci.yml).

Database integration tests run against disposable stores. Set `TRACEPILOT_INTEGRATION_TEST=1` and `TRACEPILOT_DATABASE_URL` to use a migrated PostgreSQL test database. Migration integration tests create and drop their own databases, so their PostgreSQL test role needs `CREATE DATABASE`. Enable ClickHouse integration with `TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1` and `TRACEPILOT_CLICKHOUSE_TEST_URL`.

### Project structure

The Go backend receives telemetry and serves the API. The React console lets you explore traces, logs, metrics, and incidents. PostgreSQL stores traces, metrics, incidents, and management data. ClickHouse stores raw logs. [Application structure](docs/STRUCTURE.md)

### Schema migrations

| Database | Commands after `migrate` | Migration files | Journal |
| --- | --- | --- | --- |
| PostgreSQL | `postgres up`, `postgres down STEPS`, `postgres status`, `postgres version` | [PostgreSQL migrations](backend/db/postgres/migrations/) | `schema_migrations` |
| ClickHouse | `clickhouse up`, `clickhouse status` | [ClickHouse migrations](backend/db/clickhouse/migrations/) | `tracepilot_schema_migrations` |

`migrate all` runs PostgreSQL, then ClickHouse, then verifies both schemas. It stops at the first failure. Each database retains its own applied versions. Rerun after resolving a connectivity error, or inspect and repair a dirty migration before retrying. Individual commands validate only their own database configuration. The legacy spelling `migrate postgres all` also selects both databases. `make migrate` is a PostgreSQL alias, and `make migrate-all` applies both schemas.

Inspect the current and target versions, pending versions, and dirty state with these read-only commands.

```powershell
go -C backend run ./cmd/tracepilot migrate postgres status
go -C backend run ./cmd/tracepilot migrate clickhouse status
```

The independent flags `TRACEPILOT_DATABASE_AUTO_MIGRATE` and `TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE` default to `false`. Setting a flag to `true` applies its pending SQL before opening listeners or starting workers. Migration failure stops startup.

> [!IMPORTANT]
> The consolidated PostgreSQL baseline supports fresh databases. Installations on earlier per-table or grouped histories need a reviewed data transfer or transition migration. PostgreSQL `000002` drops the legacy `logs` table. Copy any retained logs to ClickHouse with the previous release's backfill command before applying it.

The PostgreSQL baseline has no down migration. Inspect rollback SQL for later versions before using it. ClickHouse has no automatic downgrade. Its first migration validates an existing log table before adopting it. Plan a separate data migration when changing an existing partition key.

Stop older backend replicas before enabling project retention. Run `migrate all` to add the PostgreSQL policy column and remove the shared ClickHouse log TTL. The ClickHouse runtime account needs `ALTER DELETE` on `tracepilot_logs` for scoped log cleanup. Existing projects inherit `TRACEPILOT_RETENTION_MAX_AGE` until their owner sets an override.

For SQL changes, add a new versioned migration, update the named queries, regenerate SQLC with `make sqlc-check`, adapt the storage layer, and test on a disposable database. Keep already-applied migration files intact. Update database constraints when changing incident statuses, severity levels, or supported webhook events.

### Contributing and security

Keep changes focused and cover behavior changes with regression tests. Update the [flow documentation](docs/FLOW.md) and [diagrams](docs/diagrams.html) when a flow changes. Update the [Postman collection](docs/tracepilot.postman_collection.json) when routes or ingestion behavior change.

Keep credentials and real telemetry out of commits. Report vulnerabilities privately through the repository's security reporting channel. In production, use HTTPS, persistent secrets, and review the destination allowlist before enabling HTTP replay.

<details>
<summary>Application releases</summary>

Application releases use SemVer tags such as `v0.1.0-beta.1`. Application tags and database migration versions advance independently. The [release workflow](.github/workflows/release.yml) runs CI and publishes Linux binaries and backend and console images with SBOMs, provenance, and keyless signatures. The console image uses the same tag and the `-console` repository suffix. Published images include source and build metadata, and `tracepilot version` reports it.

When preparing a release, synchronize the private frontend package and lockfile if needed with `npm --prefix frontend version 0.1.0-beta.1 --no-git-tag-version`. Use a new version for each publication.

</details>

## MVP scope and release criteria

The main journey is project setup → token creation → telemetry ingest → failed trace and correlated logs → incident investigation.

- Pass Go and frontend checks, both database integration suites, and the browser smoke test.
- Rehearse fresh installation, restart with existing data, migration recovery, and restoration of both databases.
- Measure a representative workload. Record throughput, rejections, query latency, resource use, and drain time.
- Verify the complete browser journey with a regular member and a second project and environment to check scope isolation.

### Measured capacity

<details>
<summary>Short-run benchmark results</summary>

A 60-second run on November 16, 2026 used 500 spans plus 500 logs per second, five console reads per second, and eight ingestion and queue workers. The backend had 2 vCPU and 1 GiB of memory, ClickHouse had 2 vCPU and 2 GiB, and PostgreSQL had 2 GiB of memory.

| PostgreSQL CPU limit | Offered records/s | Failed ingest requests | Failed reads | Read p95 |
| --- | --- | --- | --- | --- |
| 2 vCPU | 500 | 0/60 | 2/300 (503) | 700 ms |
| 2 vCPU | 1,000 | 2/118 (503) | 101/300 (503) | 3.621 s |
| 4 vCPU | 1,000 | 0/118 | 0/300 | 689 ms |

The 4 vCPU run recorded p95 of 940 ms for traces, 942 ms for logs, and 1.183 s for topology. An earlier ingest-oriented run on November 12 reached 1,000 records/s without failures and with p95 at or below 1 s, while concurrent console reads saw 503 responses. Under 1,500 records/s, the backend drained in about 1.2 s. Readiness recovered around 30 s after a stopped ClickHouse instance returned. These runs cover short workloads with limited retained data.

Repeat the workload on disposable data after changing query windows, hardware, or data volume.

```powershell
go -C backend run ./cmd/loadgen -spans-per-second 500 -logs-per-second 500 -spans-per-request 512 -logs-per-request 512 -reads-per-second 5 -duration 60s
```

The tool creates its own project, environment, and token. Use `-email` and `-password` for an account with permission to create projects and `-metrics-token` for a protected metrics endpoint. Keep `TRACEPILOT_DATABASE_MAX_CONNECTIONS` at least twice the ingestion worker count plus two.

</details>

### Development roadmap

| Priority | Work |
| --- | --- |
| P0 | Validate the MVP across CI, the browser journey, migration recovery, and representative workloads. |
| P1 | Improve OpenTelemetry onboarding and add account recovery with expiring, single-use tokens. |
| P2 | Add scoped incident rules, evaluation previews, and notification routing controls. |
| P3 | Profile query and ingestion bottlenecks, then add capacity where repeatable measurements justify it. |

## Reference

- [Application flow](docs/FLOW.md)
- [Diagrams and searchable source map](docs/diagrams.html) (open in a browser)
- [Postman API collection](docs/tracepilot.postman_collection.json)
- [MIT license](LICENSE)

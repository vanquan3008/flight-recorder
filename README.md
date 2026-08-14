# Flight

Incident-centric observability and production replay for OpenTelemetry workloads.

> **Project status: Phase 0, Phase 1, and Phase 2 complete; not production-ready as a full incident platform yet.** OTLP traces/logs, bounded normalization and redaction, durable batch persistence, PostgreSQL lifecycle, health/readiness, Prometheus metrics, Docker, and CI are implemented and tested. Incident correlation, authentication, management APIs, capture, and replay remain delivery phases in [PLAN.md](docs/PLAN.md), not hidden stubs.

## Why Flight exists

Logs, traces, and HTTP telemetry are evidence. The incident is the product.

Most observability workflows start with an error and leave an engineer to search logs, find a trace, reconstruct request context, guess at the cause, and manually reproduce it. Flight is designed to turn correlated production evidence into one incident and, when safe request capture is enabled, a portable replay artifact.

```text
production failure
    -> normalized telemetry
    -> correlated incident
    -> inspect timeline
    -> replay against local or staging
    -> compare result
    -> verify the fix
```

Flight is not an API gateway, logging SDK, metrics dashboard, ELK replacement, or Grafana replacement. It accepts standard OpenTelemetry data and can run beside an existing observability stack.

## Design principles

- OpenTelemetry-first; proprietary logging calls are not required.
- A modular Go monolith until measured scale requires another topology.
- One canonical event model after ingestion; OTLP protobuf types do not leak into the domain.
- Redact sensitive data before persistence.
- Bound every ingestion queue and goroutine lifecycle.
- Keep expensive correlation and incident work off the OTLP response path.
- Use PostgreSQL for V1 metadata and events, filesystem storage for bounded payload artifacts.
- Make replay deny-by-default and resistant to SSRF and credential reuse.
- Never claim a feature works before its end-to-end path is tested.

## Architecture

```mermaid
flowchart LR
    App[Application] --> OTel[OpenTelemetry SDK / Collector]
    OTel --> Receiver[OTLP gRPC :4317 / HTTP :4318]
    Receiver --> Limits[Byte + record limits]
    Limits --> Normalize[Normalize + validate + redact]
    Normalize --> Queue[Bounded queue]
    Queue --> Batch[Fixed batch workers]
    Batch -->|transaction commit| PG[(PostgreSQL events)]
    PG -->|ACK success| Receiver
    Limits -->|invalid records| Partial[OTLP partial success]
    Queue -->|full / timeout| Retry[Retryable response]
    PG -. Phase 3 .-> Correlate[Correlation workers]
    Correlate -.-> Detect[Incident detector]
    Detect -.-> Incident[(Incident)]
    Incident -.-> API[Management API / CLI]
    Incident -. safe captured request .-> Replay[Replay engine]
    Replay -. allowlisted target .-> Target[Local / staging target]
```

The solid path is implemented in Phase 2. Dashed paths begin in Phase 3. OTLP protobuf types stop at `internal/ingest/otlp`; the queue and PostgreSQL layers only receive canonical `telemetry.Event` values. An OTLP success is returned only after its accepted events commit to PostgreSQL.

## What works now

- `flight serve` starts a hardened HTTP server.
- `GET /health/live` reports process liveness.
- `GET /health/ready` verifies the management/OTLP listeners, ingestion pipeline, PostgreSQL connectivity, and the expected clean schema version.
- `GET /metrics` exposes a dedicated low-cardinality Prometheus registry.
- OTLP traces and logs are accepted over gRPC on `:4317` and OTLP/HTTP protobuf or JSON on `:4318`.
- Compressed and decoded request bytes, records, attributes, nesting, strings, queue capacity, workers, batches, retries, and shutdown are bounded.
- Resource/scope/span/log records normalize into one canonical event model; malformed records produce OTLP partial success rather than a process failure.
- Sensitive attribute keys are redacted case-insensitively before an event can enter the queue.
- Batch workers retry a finite number of times and acknowledge requests only after an idempotent PostgreSQL transaction commits.
- A statically configured project/environment identity is enforced for the trusted Phase 2 Collector boundary.
- Safe incoming request IDs are preserved; invalid or missing IDs are replaced and returned as `X-Request-ID`.
- Access logs are structured JSON and avoid headers and bodies.
- SIGINT/SIGTERM initiates bounded graceful shutdown.
- Versioned YAML configuration is strict, size-limited, environment-overridable, and validated before startup.
- A bounded pgx connection pool is configured from validated settings.
- `flight migrate up|down STEPS|version` runs reviewed migrations explicitly.
- The canonical `telemetry.Event` model and deterministic V1 incident candidate detector are implemented and unit-tested.
- The initial PostgreSQL schema and indexes are versioned in `migrations/`.

The current scaffold deliberately returns `404` for `/api/v1/incidents`: there is no fake in-memory incident API masquerading as the production path.

## Repository layout

```text
.
|-- cmd/flight/                 # One binary; server now, CLI commands incrementally
|-- internal/
|   |-- config/                 # Validated runtime configuration
|   |-- httpapi/                # HTTP lifecycle and cross-cutting middleware
|   |-- ingest/otlp/            # OTLP gRPC/HTTP transport and normalization
|   |-- incident/               # Failure detection and fingerprinting
|   |-- metrics/                # Prometheus self-observability
|   |-- migrations/             # In-binary migration command
|   |-- pipeline/               # Bounded admission, batching, commit acknowledgements
|   |-- postgres/               # pgx pool, readiness, and event persistence
|   `-- telemetry/              # Canonical event model
|-- examples/demo-service/      # Instrumented synthetic trace producer
|-- configs/                    # Versioned example configuration
|-- migrations/                 # Explicit, versioned PostgreSQL migrations
|-- docs/
|   |-- adr/                    # Small architecture decisions
|   |-- architecture.md        # Runtime boundaries, durability, scaling
|   |-- configuration.md       # YAML and environment reference
|   |-- deployment.md          # Release, ports, probes, container contract
|   |-- development.md         # Local workflow and verification
|   |-- event-model.md         # Canonical telemetry event
|   |-- incident-model.md      # Phase 3 incident rules
|   |-- ingestion.md           # Implemented Phase 2 data path
|   |-- PLAN.md                 # Phased implementation and acceptance gates
|   |-- replay.md               # Disabled replay contract and gates
|   |-- security.md             # Threat model and release gates
|   `-- storage.md              # PostgreSQL and future blob boundary
|-- .github/workflows/ci.yml
|-- compose.yaml
|-- Dockerfile
|-- Makefile
`-- README.md
```

Packages are added only when a production vertical slice needs them. There are no empty `replay`, `capture`, `storage`, `proxy`, or language-integration packages.

## Quick start

### Docker (recommended)

Requirements: Docker Engine with Compose v2.

```bash
docker compose up --build -d postgres
docker compose run --rm migrate
docker compose up -d flight
curl -i http://localhost:8080/health/live
curl -i http://localhost:8080/health/ready
```

Expected responses are `{"status":"ok"}` for liveness and `{"status":"ready"}` for readiness:

```json
{"status":"ok"}
```

```json
{"status":"ready"}
```

Send a synthetic failing trace and verify that it was persisted:

```bash
docker compose up -d demo
curl -i http://localhost:8090/error
docker compose exec postgres psql -U flight -d flight \
  -c "SELECT event_type, service_name, http_status, status FROM events ORDER BY occurred_at DESC LIMIT 5;"
```

Stop the stack without deleting PostgreSQL data:

```bash
docker compose down
```

Delete the local development volume only when you intentionally want a clean database:

```bash
docker compose down --volumes
```

### Native Go

Requirements: Go 1.25 or a compatible newer release.

```bash
export FLIGHT_DATABASE_URL='postgres://flight:flight-dev-only@localhost:5432/flight?sslmode=disable'
go run ./cmd/flight migrate up
go run ./cmd/flight serve --config configs/flight.example.yaml
```

In another terminal:

```bash
curl http://localhost:8080/health/live
```

## Configuration

Configuration format version `1` supports strict YAML plus environment overrides. Precedence is CLI `--config` selection, environment, YAML, then defaults. Unknown fields, multiple YAML documents, oversized files, and invalid bounds fail before the server listens. See [configuration reference](docs/configuration.md).

| Variable | Default | Purpose |
|---|---:|---|
| `FLIGHT_HTTP_ADDRESS` | `:8080` | Management HTTP listen address |
| `FLIGHT_HTTP_READ_HEADER_TIMEOUT` | `5s` | Maximum time to read request headers |
| `FLIGHT_HTTP_READ_TIMEOUT` | `15s` | Maximum request read time |
| `FLIGHT_HTTP_WRITE_TIMEOUT` | `30s` | Maximum response write time |
| `FLIGHT_HTTP_IDLE_TIMEOUT` | `60s` | Keep-alive idle timeout |
| `FLIGHT_HTTP_MAX_HEADER_BYTES` | `1048576` | Maximum request header bytes |
| `FLIGHT_HTTP_MANAGEMENT_MAX_BODY_BYTES` | `1048576` | Maximum management request body bytes |
| `FLIGHT_INGESTION_GRPC_ADDRESS` | `:4317` | OTLP/gRPC traces and logs listener |
| `FLIGHT_INGESTION_HTTP_ADDRESS` | `:4318` | OTLP/HTTP traces and logs listener |
| `FLIGHT_INGESTION_PROJECT_ID` | development UUID | Trusted Collector project identity |
| `FLIGHT_INGESTION_ENVIRONMENT_ID` | development UUID | Trusted Collector environment identity |
| `FLIGHT_INGESTION_MAX_REQUEST_BYTES` | `4194304` | Compressed and decoded OTLP request limit |
| `FLIGHT_INGESTION_MAX_RECORDS_PER_REQUEST` | `10000` | Trace/log records accepted per request |
| `FLIGHT_INGESTION_QUEUE_SIZE` | `4096` | Maximum events waiting for a batch worker |
| `FLIGHT_INGESTION_QUEUE_MAX_BYTES` | `67108864` | Maximum estimated bytes admitted but not yet committed |
| `FLIGHT_INGESTION_ENQUEUE_TIMEOUT` | `250ms` | Maximum bounded queue admission wait |
| `FLIGHT_INGESTION_BATCH_SIZE` | `256` | Maximum events per PostgreSQL transaction |
| `FLIGHT_INGESTION_WORKERS` | `2` | Fixed event batch worker count |
| `FLIGHT_INGESTION_REDACTED_KEYS` | empty | Additional comma-separated exact attribute keys to redact |
| `FLIGHT_DATABASE_URL` | required | PostgreSQL connection URL; inject as a secret |
| `FLIGHT_DATABASE_MAX_CONNECTIONS` | `20` | pgx maximum connections |
| `FLIGHT_DATABASE_MIN_CONNECTIONS` | `2` | pgx minimum connections |
| `FLIGHT_DATABASE_READINESS_TIMEOUT` | `2s` | PostgreSQL/schema readiness deadline |
| `FLIGHT_MIGRATIONS_PATH` | `./migrations` | Reviewed SQL migration directory |
| `FLIGHT_SHUTDOWN_TIMEOUT` | `15s` | Graceful shutdown deadline |
| `FLIGHT_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `FLIGHT_HEALTHCHECK_URL` | `http://127.0.0.1:8080/health/ready` | Container probe target; change when using a non-default port |

Invalid values fail startup with exit code `2`. Never put credentials in committed config files.

## PostgreSQL migrations

Migrations never run automatically during server startup. This keeps schema changes explicit and allows a deployment job to run them once before rolling out compatible application instances.

For local development:

```bash
docker compose up -d postgres
docker compose run --rm migrate
docker compose run --rm migrate migrate version
```

The initial schema covers projects, environments, services, normalized events, incidents, occurrences, incident timelines, payload references, replay executions, and hashed API-key records. Feature code will adopt these tables phase by phase. See [PLAN.md](docs/PLAN.md#database-model) for invariants and deduplication semantics.

## OpenTelemetry integration

An existing Collector can fan out traces and logs to Flight without replacing the current backend:

```yaml
exporters:
  otlp/flight:
    endpoint: flight:4317
    tls:
      insecure: true # development only

service:
  pipelines:
    traces:
      exporters: [otlp/flight, otlp/existing-backend]
    logs:
      exporters: [otlp/flight, otlp/existing-backend]
```

Production deployments must use TLS or a trusted private transport. Metrics ingestion is outside V1.

OTLP/HTTP clients may send protobuf or JSON to `POST /v1/traces` and `POST /v1/logs` on port `4318`. Gzip is accepted with both compressed and decompressed size limits. The Phase 2 identity comes from Flight configuration, never from an untrusted OTLP body or header; authenticated per-project mapping arrives in Phase 4.

## Create the first incident (target workflow)

This path becomes available at the Phase 3 acceptance gate:

```bash
docker compose up -d
curl http://localhost:8090/error
flight incidents list --server http://localhost:8080
```

Phase 3 will claim the already durable event, detect the deterministic failure, deduplicate it in a transaction, correlate trace/request events, and return the incident from `GET /api/v1/incidents`.

## Replay an incident (target workflow)

Replay is Phase 6 and remains disabled until capture redaction and target policy pass their security gates:

```bash
flight replay INC-01J... \
  --target http://localhost:8090
```

By default Flight will strip credentials, reject public or production targets, block link-local/cloud metadata addresses, revalidate redirects, cap request and response sizes, enforce a timeout, and record an audit entry. It will never replay captured production credentials automatically.

## API

Implemented now:

| Method | Path | Authentication | Meaning |
|---|---|---|---|
| `GET` | `/health/live` | none | Process liveness only |
| `GET` | `/health/ready` | none | Listener/pipeline availability plus PostgreSQL and clean schema readiness |
| `GET` | `/metrics` | network policy | Prometheus self-metrics |
| OTLP/gRPC | `:4317` traces/logs services | trusted network/config identity | Durable trace and log ingestion |
| `POST` | `:4318/v1/traces` | trusted network/config identity | OTLP/HTTP protobuf or JSON traces |
| `POST` | `:4318/v1/logs` | trusted network/config identity | OTLP/HTTP protobuf or JSON logs |

Planned versioned management endpoints include incident list/detail/timeline and replay execution under `/api/v1`. Cursor pagination is based on a stable `(timestamp, id)` tuple. Errors use a stable code, a safe message, and the management request ID; internal Go errors are never returned.

## Development and verification

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Or with Make:

```bash
make fmt-check vet test test-race build
```

CI executes format validation, `go vet`, race-enabled tests, build, Staticcheck, clean PostgreSQL migration, server probes, and a demo trace-to-event smoke test. PostgreSQL integration uses real PostgreSQL; SQLite is not an accepted substitute.

## Container posture

The Dockerfile uses a separate build stage and a distroless non-root runtime. Compose runs the service with a read-only root filesystem and a bounded init process. Management port `8080` and OTLP ports `4317`/`4318` are exposed. The process stops OTLP admission, drains accepted event batches, then closes PostgreSQL within the shutdown deadline.

Before the first production release the container gate also verifies OCI health behavior, writable blob mounts, CA certificates for replay, SBOM generation, image vulnerability scanning, resource limits, and clean shutdown while queues contain data.

## Security

Captured telemetry may contain credentials, payment data, personal data, and internal topology. Phase 2 already bounds requests and redacts default/custom sensitive attribute keys before queueing. Public production exposure remains blocked until these later controls are implemented and tested:

- API-key authentication with only hashed keys at rest and explicit `read`, `replay`, and `admin` scopes.
- Bounded payload reads, content-type validation, truncation metadata, and no payload logging.
- Project ID enforcement in every repository method and foreign-key relationship.
- Filesystem blob keys generated by Flight; no user-controlled paths.
- Replay allowlists, resolved-IP checks, redirect revalidation, metadata endpoint denial, credential stripping, timeouts, and audit records.
- TLS guidance and separate authentication policy for ingestion and management APIs.

Read the [threat model](docs/security.md) before implementing capture or replay. Report vulnerabilities according to [SECURITY.md](SECURITY.md).

## Delivery roadmap

1. ~~Architecture and production foundation.~~ Complete.
2. ~~OTLP traces/logs, normalization, redaction, bounded batch persistence.~~ Complete.
3. Deterministic incident engine, correlation, timeline, demo service.
4. Authenticated management API and CLI.
5. Safe bounded HTTP capture and filesystem blob storage.
6. Deny-by-default replay with audit.
7. JSON semantic comparison and text fallback.
8. Load, race, outage, migration, security, container, and documentation hardening.

Each phase has measurable entry/exit criteria in [PLAN.md](docs/PLAN.md). Kafka, Redis, ClickHouse, Grafana, a service mesh, Helm, eBPF, AI correlation, and external observability adapters are not V1 dependencies.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Never use real customer telemetry in tests, examples, issues, or pull requests.

## License

Flight is licensed under [Apache License 2.0](LICENSE).

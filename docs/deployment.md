# Deployment

Phase 2 ships one server process with management and OTLP listeners plus PostgreSQL. Filesystem blob storage is not active yet.

## Release sequence

1. Back up PostgreSQL according to the environment policy.
2. Run `flight migrate up` once as a dedicated deployment job.
3. Verify `flight migrate version` reports the expected clean schema.
4. Roll out Flight instances.
5. Route traffic only after `/health/ready` returns `200`.

The application never runs migrations on startup. Liveness reports only process health. Readiness verifies that the management and both OTLP listeners are accepting work, the ingestion pipeline is open, PostgreSQL is reachable, and schema version `1` is present and not dirty.

## Container contract

- Runs as the distroless `nonroot` user.
- Supports a read-only root filesystem.
- Listens on management port `8080`, OTLP/gRPC `4317`, and OTLP/HTTP `4318` by default.
- Contains `/flight` and reviewed SQL under `/migrations`.
- Handles SIGTERM by stopping listeners and draining accepted ingestion batches within a configured shutdown deadline.
- Emits structured JSON logs to stdout/stderr.

Inject `FLIGHT_DATABASE_URL` as a secret. Use TLS (`sslmode=verify-full`) outside trusted local development. Apply CPU/memory limits and keep PostgreSQL connection settings inside the database budget.

## Probe policy

| Probe | Path | Meaning |
|---|---|---|
| Liveness | `/health/live` | Process can serve HTTP; never depends on PostgreSQL |
| Readiness | `/health/ready` | Management/OTLP listeners and pipeline accept work; PostgreSQL responds; schema is current |

Prometheus scrapes `/metrics`. Restrict that endpoint with network policy or an internal listener at the deployment boundary until separate metrics authentication/listening is implemented.

Kubernetes manifests remain deferred until resource measurements exist; copying premature manifests would encode unverified resources and replica behavior. Phase 2 supports one replica operationally. Multiple replicas wait for Phase 3 durable work claims and cross-replica incident deduplication tests.

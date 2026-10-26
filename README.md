# TracePilot

Self-hosted OpenTelemetry observability for small teams. TracePilot accepts
OTLP traces, application logs and metrics, correlates them with incidents and
service topology, and ranks RCA hypotheses with inspectable evidence.

**Status: beta.** Production acceptance requires the checks in
[Operations](docs/OPERATIONS.md). Traces/logs acknowledge committed PostgreSQL
transactions. Metrics acknowledge queue admission and remain best-effort.

## Run locally

Requires Docker Compose v2. For frontend development, use the Node version in
web/.nvmrc; Go follows backend/go.mod.

```powershell
npm --prefix web ci
npm --prefix web run build
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
# In .env: use COMPOSE_PROFILES=local and DB host postgres for Compose.
docker compose up -d --wait postgres
docker compose run --build --rm migrate
docker compose up --build -d tracepilot
docker compose run --rm tracepilot mockdata
```

Open http://localhost:8080 and connect with the development-only bootstrap key:

```text
tracepilot-development-bootstrap-key-000000
```

Create a dedicated ingestion key for your application. The console provides
OTLP configuration. Use a read key for everyday investigation.
OTLP endpoints: gRPC :4317; HTTP :4318 with /v1/traces, /v1/logs, /v1/metrics.
Management API: /api/v1; readiness: /health/ready; liveness: /health/live.

Configuration lives in one repository-root .env; .env.example is its template.
The single compose.yaml injects that file into containers. The local profile
enables PostgreSQL; clear COMPOSE_PROFILES and configure an external database,
production TLS and a release image digest for production. Native Go uses DB
host localhost instead of postgres and reads .env (or ../.env with go -C backend).
See [Development](docs/DEVELOPMENT.md) for precedence and [Operations](docs/OPERATIONS.md)
for production values. Keep your existing .env when updating the repository.

## Console

Overview (traffic, errors, latency, incidents needing action), trace explorer
with span waterfall, log explorer with trace/span correlation, service topology
graph, incidents with RCA hypotheses, and settings for API keys and webhooks.
Light and dark themes follow the system and can be switched manually.

## How it works

SDK/Collector → authenticated OTLP receiver → validation/redaction →
bounded pipeline → PostgreSQL transaction → ACK → REST/embedded React console.

Error incidents and notification outbox entries are stored with their events.
A separate scheduled detector compares recent service/dependency/metric signals
with a baseline. RCA output is a ranked hypothesis, not proof of causality.
Webhooks are optional, signed, encrypted at rest, and subject to outbound
network controls. Replay is disabled by default.

One Go process and one PostgreSQL database are the current deployment target.
Workflow packages live in internal/application; pure rules/models in
internal/domain; protocol adapters in internal/ingest and internal/transport;
SQL/transactions in internal/storage. The CLI wires these through small ports.
React features live in web/src/features.

## Current documentation

| Guide | Purpose |
| --- | --- |
| [Architecture and flow](docs/ARCHITECTURE.md) | Source ownership, dependency rules, runtime sequence and design reasons |
| [Development](docs/DEVELOPMENT.md) | Native/Compose setup, unit/integration/browser checks and generated assets |
| [Operations](docs/OPERATIONS.md) | Production config, migration, backup/restore, load, outage and shutdown acceptance |
| [Source review](docs/REVIEW.md) | Verified findings, fixes, test results and remaining limitations |
| [Roadmap and alternatives](docs/ROADMAP.md) | Priorities, implementation choices and acceptance criteria |

The guides are written in Vietnamese for the repository owner.
[CONTRIBUTING.md](CONTRIBUTING.md) describes contribution checks;
[SECURITY.md](SECURITY.md) describes private vulnerability reporting.

## Production limits

- The current schema is baseline 000001_init, version 1. It does not upgrade
  databases using the former migration history. Preserve existing data and plan
  a tested data migration before switching such a database.
- Configure authenticated TLS listeners, PostgreSQL verify-full with a trusted
  CA, private binds, an immutable image digest, and an explicit data policy.
- Metrics can be lost after ACK on storage failure. Byte budgets are estimates
  per pipeline; they do not cap total process RSS.
- Key-based attribute redaction does not remove every secret from free text.
- Telemetry retention defaults to disabled. Incident/evidence/audit/delivery
  history needs a separate policy. No production throughput SLO is certified.

Run the production acceptance drills on staging before admitting real traffic.
License: [MIT](LICENSE).

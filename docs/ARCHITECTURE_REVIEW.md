# Architecture review and targeted refactor

Review date: 2026-09-09

This review examines runtime responsibility and dependency direction, not only
folder names. The changes in this document deliberately avoid a broad rewrite:
the existing service is a pragmatic modular monolith and its important
transactional boundaries should remain intact.

## Runtime map

```text
cmd/tracepilot (composition root)
  -> config, migrations, postgres, metrics
  -> OTLP receiver / syslog receiver -> ingestion pipeline -> PostgreSQL
  -> management HTTP API -------------------------------> PostgreSQL
  -> security detection worker -------------------------> PostgreSQL transaction
  -> notification delivery worker <--------------------- PostgreSQL outbox
  -> retention worker ----------------------------------> PostgreSQL
  -> replay service -> replay executor + PostgreSQL execution record

web/src app shell -> feature pages -> shared API client -> management HTTP API
```

## Ownership and dependency map

| Area | Owns | May depend on | Must not own |
| --- | --- | --- | --- |
| `internal/domain/*` | Stable telemetry, security, incident, replay and webhook value rules | Go standard library and other domain values | Configuration, SQL, HTTP, listeners |
| `internal/ingest/otlp` and `internal/ingest/syslog` | Protocol authentication, size validation and normalization into `telemetry.Event` | Config, pipeline port, metrics | PostgreSQL queries |
| `internal/ingest/pipeline` | Bounded admission, batching, acknowledgement and retry of durable event writes | `StoreFunc`, canonical telemetry, metrics | OTLP, syslog, SQL implementation |
| `internal/security` | Parse raw syslog/CEF/LEEF records and attach a validated security value to the canonical event | Domain telemetry/security | Storage and transport |
| `internal/storage/postgres` | PostgreSQL pool lifecycle, SQL, idempotency, transactional persistence and outbox leasing | Domain/query/config values, pgx | HTTP handlers, listeners, replay network calls |
| `internal/transport/httpapi` | HTTP input/output, authorization scope checks, status mapping and OpenAPI route manifest | Narrow backend ports, domain/query values | Concrete PostgreSQL adapter |
| `internal/replay` | Allow-list/SSRF policy, bounded HTTP execution and execution state orchestration | Replay store port and capture/domain values | HTTP management transport and SQL details |
| `internal/notification` | Signed outbound delivery, bounded request body handling and polling coordination | Delivery store port and notification domain values | SQL details |
| `web/src` | Browser routing, session-only API-key state, feature presentation and query caching | Management API contract | Backend storage/domain implementation |

The dependency guard in `internal/architecture/dependencies_test.go` enforces
the most important edges: domain cannot import adapters, transport and ingest
cannot import PostgreSQL, and PostgreSQL cannot import replay.

## Refactors applied

### 1. Configuration is no longer mutated after validation

Previously, `runServer` copied `notifications.encryption_key` into a hidden
field on `DatabaseConfig` after `config.Load` had already validated the
configuration. That made the validated value differ from the runtime value and
made secret ownership implicit.

`postgres.OpenWithWebhookEncryptionKey` now receives the webhook encryption
key explicitly from the composition root. `DatabaseConfig` contains only
database settings again, while the PostgreSQL adapter still keeps the decoded
key private and never exposes it through HTTP. The ordinary `postgres.Open`
path remains intentionally incapable of encrypting webhook secrets.

### 2. Notification worker has one complete store contract

`notification.Service` previously accepted a store that could record an
attempt, then used a runtime type assertion before it could claim delivery
work. A wrongly wired service would therefore fail only when its worker began.

The `notification.Store` contract now requires both operations. The compiler
checks that every service instance can run its complete lifecycle; the
separate runtime-only `RetryStore` capability was removed. The interface is
still small and consumer-owned, so no repository abstraction was introduced.

## Deliberate non-changes

- `postgres.Database` exposes many methods, but it is an infrastructure
  facade over one pool rather than a domain service. Its files are already
  split by persistence concern and callers consume narrow interfaces. Splitting
  it into many wrapper structs would add names and wiring without reducing SQL
  coupling.
- Security detection remains a PostgreSQL transaction because its checkpoint,
  advisory lock, rule lookup, incident update, detection insert and outbox
  intent must commit atomically. `security.Matches` remains pure domain logic.
- The ingestion pipeline remains the durability boundary: OTLP/syslog success
  is returned only after `StoreEvents` commits. Metrics intentionally use a
  documented best-effort path; changing that requires a metrics durability SLO
  and load benchmark.
- `runServer` remains the composition root. Its `build*` helpers are focused
  wiring functions and do not contain hidden business policy, so another
  bootstrap package would not reduce complexity.

## Remaining risks and next work

| Priority | Finding | Recommended next step |
| --- | --- | --- |
| P2 local verification | PostgreSQL integration tests need a disposable migrated database, so they are intentionally opt-in on a developer machine. | The GitHub Actions test job already runs them with PostgreSQL, the race detector and a browser smoke test; use Docker locally when reproducing an integration failure. |
| P2 scaling | Security rule evaluation scans and evaluates in the database transaction. It is correct and serialized, but rule/event volume should be measured. | Add a benchmark and query-plan/lag alert before changing the algorithm or introducing a separate worker store. |
| P2 operations | Webhook retry delay and maximum attempts are intentionally fixed in the outbox implementation. | Make them config only after operational requirements call for per-environment tuning; keep exponential backoff/idempotency requirements explicit if changed. |
| P2 API evolution | `httpapi.Backend` composes several narrow ports. | Keep adding methods to the existing domain-scoped interfaces; split a handler-local port only when a new caller needs a materially smaller fake. |
| P2 UI quality | The React structure is appropriately small: app shell, feature pages and shared primitives. | Add route-level tests only as interactions or data states grow; avoid a client-side domain layer that duplicates the server. |

## Verification

The following checks passed after the refactor:

```powershell
Set-Location backend
go test ./...
go vet ./...
go build ./cmd/tracepilot ./cmd/tracepilot-agent
go run honnef.co/go/tools/cmd/staticcheck@2026.1 ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

Set-Location ../web
npm run check
npm test
npm run build
```

The Go integration tests remain skipped unless
`TRACEPILOT_INTEGRATION_TEST=1` and `TRACEPILOT_DATABASE_URL` point to a
migrated disposable PostgreSQL database. GitHub Actions supplies that database
and runs `go test -race` plus the browser smoke test. A local Windows setup
also needs a C compiler before the Go race detector can run.

## Assessment

Current assessment: **strong codebase; production readiness depends on
operational evidence**.
Layer boundaries, canonical telemetry ownership, transaction durability,
graceful listener shutdown, bounded queues, security policy and frontend
feature separation are sound. An honest “10/10” requires evidence from
production-scale load, backup/restore and outage drills; that is an operations
claim, not a refactor that can be truthfully made in source code alone.

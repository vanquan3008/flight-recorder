# TracePilot operations runbook

This runbook is the production companion to [DEVELOPMENT.md](DEVELOPMENT.md).
The first production topology is one immutable TracePilot image, a private
managed PostgreSQL database, and a TLS reverse proxy. PostgreSQL is never
published by the production Compose file.

## Deploy and verify

1. Copy `deploy/.env.production.example` to a secret-manager controlled file.
2. Replace the image with a release digest, database URL, bootstrap key, TLS
   directory, and trusted-network listener binds.
3. Run `tracepilot migrate up` as a one-shot job, then start the image.
4. Require `/health/ready` before sending traffic and check `/metrics` with the
   read scope. Confirm the database schema version equals the binary's version.

When direct Syslog is enabled, mount a CA that signs sender certificates as
`ca.crt`; production configuration rejects Syslog without TLS and client-CA
verification. Before a rotation, run `scripts/cert-check.ps1` for expiry, SAN
and issuer-chain evidence, then restart/roll the replica so the new key pair is
loaded.

Do not use `migrate down` as an application rollback. Roll back the image only
when its schema is compatible; otherwise restore the tested database backup or
deploy the forward-compatible migration first.

## Backup and restore acceptance drill

Run a full `pg_dump -Fc` at least daily, retain it according to the data policy,
and monitor backup age/size. Once per release:

```bash
pg_dump -Fc "$TRACEPILOT_DATABASE_URL" > tracepilot-$(date -u +%Y%m%dT%H%M%SZ).dump
createdb tracepilot_restore
pg_restore --exit-on-error --clean --if-exists -d tracepilot_restore tracepilot.dump
TRACEPILOT_DATABASE_URL="$TRACEPILOT_RESTORE_URL" tracepilot migrate version
TRACEPILOT_DATABASE_URL="$TRACEPILOT_RESTORE_URL" tracepilot healthcheck
```

The drill passes only when row counts for events, spans, logs, security events,
incidents, metrics and webhook deliveries match the source, readiness is green,
and a sample API query returns the expected project-scoped data. Destroy the
restore database after the evidence is recorded.

## Outage and recovery drill

- Stop PostgreSQL or revoke its network route for five minutes. Ingest clients
  must receive bounded errors; the server must not acknowledge uncommitted
  records, and queue depth/DB error metrics must rise.
- Restore PostgreSQL. Confirm readiness, bounded queue drain, and no duplicate
  event rows. The agent may replay its disk spool; verify its checkpoint moves
  only after an accepted OTLP response.
- For webhook outage, return HTTP 503 from a test endpoint. Deliveries must move
  `pending -> sending -> pending` and finally `failed` after three attempts;
  restart TracePilot and confirm stale leases are reclaimed.

## Migration rollback drill

On a disposable clone, record the current version, apply one migration, run
`migrate down 1`, and verify the previous image rejects the newer schema until
the migration is reapplied. Test a dirty migration marker and confirm both
`migrate version` and server readiness fail closed. Capture command output and
the exact release/image digest in the change record.

The repository includes a guarded wrapper for the disposable database portion:

```powershell
.\scripts\upgrade-rollback-drill.ps1 `
  -DatabaseUrl $env:TRACEPILOT_DISPOSABLE_DATABASE_URL `
  -AllowDestructive
```

It performs `migrate up`, `migrate down 1`, and `migrate up` again, printing the
schema version after each step. Application compatibility and N/N-1 policy
still need verification with a real previous image in staging.

## Load and capacity drill

Run `make bench-pipeline` for a CPU baseline and `make bench-postgres` against a
disposable database. In staging, run a five-minute OTLP mixed traces/logs/metrics
load at 2x expected peak. Record p50/p95/p99 ingest acknowledgement latency,
throughput, queue depth/bytes, DB pool saturation, WAL rate, CPU, memory, and
disk/index growth. The release gate is the agreed SLO, not a fixed benchmark
number; abort on unbounded queue growth, rising failed batches, or data loss.

For a repeatable HTTP soak with explicit thresholds:

```powershell
.\scripts\load-test.ps1 -DurationMinutes 5 -Concurrency 8 `
  -HealthUrl http://localhost:8080/health/ready -MaxP95Ms 2000
```

Capture representative read-only query evidence with:

```powershell
.\scripts\query-plan.ps1 -DatabaseUrl $env:TRACEPILOT_DATABASE_URL `
  -ProjectId $projectId -EnvironmentId $environmentId -PsqlContainer tracepilot-postgres-1 `
  -Output .\query-plan.txt
```

## Webhook security and operations

Set `TRACEPILOT_NOTIFICATIONS_ENABLED=true` only with a secret-manager supplied
`TRACEPILOT_WEBHOOK_ENCRYPTION_KEY` (32 bytes, hex or base64). Endpoint URLs
must be HTTPS. The worker resolves DNS at connection time and rejects private,
loopback, link-local, shared, documentation and metadata ranges; redirects are
not followed. Rotate a receiver secret with the rotate endpoint and update the
receiver before enabling traffic again.

## Alerts

Alert on database readiness false, ingestion queue saturation, storage errors,
security detection lag/checkpoint age, retention backlog, webhook delivery
attempt failures (`tracepilot_webhook_delivery_attempts_total{outcome="failure"}`),
webhook claim errors, backup age, disk usage and TLS expiry. Every alert should
link to this runbook and include project/environment scope.

For shutdown visibility, watch `tracepilot_ingestion_inflight_events` and alert
on `tracepilot_ingestion_shutdowns_total{outcome="timeout"}`. A successful
graceful drain is recorded with `outcome="completed"`.

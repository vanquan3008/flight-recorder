# Operations runbook

Flight requires PostgreSQL backups; telemetry durability is bounded by the last successful database backup.

## Backup and restore

Run a logical backup against the production database and retain it outside the deployment host:

```sh
pg_dump --format=custom --file=flight-$(date +%F).dump "$FLIGHT_DATABASE_URL"
```

Test restore in an empty disposable database before accepting a release:

```sh
createdb flight_restore_test
pg_restore --clean --if-exists --dbname=flight_restore_test flight-YYYY-MM-DD.dump
flight migrate version
```

## Upgrade and rollback

1. Back up the database and verify the restore procedure.
2. Deploy the new image without changing configuration version.
3. Run `flight migrate up`; wait for `/health/ready`.
4. Verify ingestion, a trace query, and database migration version.
5. Roll back the image only if no irreversible migration has run. Otherwise restore the tested backup into a new database, validate it, then switch traffic.

## Release gates

- `go test ./...`, `go vet ./...`, and `go build ./...` pass in Linux CI.
- PostgreSQL migration up/down/up, integration, race, and browser E2E jobs pass.
- A backup/restore drill is recorded for the target PostgreSQL version.
- TLS terminates before Flight; bootstrap keys are supplied only by a secret manager.
- Replay allowlists and agent file permissions are reviewed.

## File agent

Run the agent with a persistent checkpoint and a bounded spool on the same
filesystem as the log source. Keep the export batch smaller than Flight's OTLP
request limit:

```sh
flight-agent --source /var/log/app.log --checkpoint /var/lib/flight/checkpoint.json \
  --disk-buffer /var/lib/flight/spool --disk-buffer-bytes 67108864 \
  --batch-bytes 3145728 --batch-lines 1000 --endpoint flight.example:4317 --tls
```

The agent exposes Prometheus counters only when `--metrics-address` is set.
Corrupt spool payloads are logged and discarded to let subsequent payloads
progress. A full spool retains the source checkpoint, so preserve the source
file until Flight recovers.

## High-volume ingestion

Load-test with representative attribute and body sizes before raising limits.
Watch `flight_ingestion_queue_depth`, `flight_ingestion_inflight_bytes`,
`flight_event_batch_duration_seconds`, `flight_event_storage_errors_total`, and
OTLP `unavailable` outcomes. A queue that stays near capacity means PostgreSQL
is not committing at the incoming rate.

Increase `ingestion.batch_size` first, then `ingestion.workers`; keep
`database.max_connections` at least two above the worker count. Stop increasing
workers when batch latency or PostgreSQL lock/I/O wait rises, since more workers
then add contention instead of throughput.

## Retention

Event retention is intentionally disabled by default. Configure
`retention.max_age` (for example `720h`) and a suitable batch size before
enabling it. Retention deletes old event rows in bounded batches; it does not
guarantee an immediate reduction in PostgreSQL's physical size. Set a volume
quota for a hard disk cap and let PostgreSQL autovacuum reclaim space.

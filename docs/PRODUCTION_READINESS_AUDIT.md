# Production readiness audit

Audit date: 2026-11-24. Scope: the Go backend (ingest, storage, workers, HTTP API), the ClickHouse and
PostgreSQL schemas, and the production compose file. The console was not changed and not audited.

## 1. Executive summary

The data path is sound. OTLP requests are acknowledged only after ClickHouse confirms the insert and
the PostgreSQL transaction for the batch's failures commits. Every log, span, metric point, incident,
and evidence row carries a deterministic ID, so a retry repeats the same rows instead of adding new ones.
Minute rollups are rebuilt from deduplicated spans, and percentiles merge t-digest states. No finding
showed acknowledged telemetry being lost, statistics double-counted, or data crossing tenants.

The audit fixed three verified defects:

| ID | Severity | Defect | Status |
| --- | --- | --- | --- |
| F1 | P1 | Repeated incident transitions with identical payloads dropped their webhook | Fixed |
| F2 | P1 | The rollup refresh fell permanently behind once more than 1,000 minutes became stale per minute, and it had no metric of its own | Fixed |
| F3 | P2 | ClickHouse failures were logged without the exception code | Fixed |

Section 5 lists the open items. Most need a measurement or a product decision before any code change.
Integration tests and the race detector could not run on the audit machine (section 8). Treat the
fixes as unit-tested and reviewed, not as verified against real databases.

## 2. Method

- Read the ClickHouse and PostgreSQL migrations, the ingest receiver, normalizers, both pipelines,
  `logs.Persist`, the ClickHouse client and its insert and read paths, incident and outbox storage,
  detection, the rollup refresh, retention, the replay and webhook HTTP clients, scope routing, the
  supervisor, and `deploy/compose.yaml`.
- Traced logs, spans, and metric points from the receiver to their tables and back through the read
  APIs.
- Checked every claim in this report against the code. Behavior that was not run is labeled as such.
- No benchmarks or query plans were produced. The audit makes no performance claim.

## 3. Architecture assessment

| Area | Assessment |
| --- | --- |
| Acknowledgment | `pipeline.Submit` waits for the per-event acknowledgment. ClickHouse inserts use `async_insert=1` with `wait_for_async_insert=1`, so success means the rows reached parts. No acknowledged batch exists only in memory. |
| Partial failure | Logs, spans, and failures are written in order. If a later write fails, the client retries the whole request. ClickHouse absorbs repeated rows through `ReplacingMergeTree` with `FINAL` reads. PostgreSQL skips events whose occurrences are already recorded. |
| Identity | Spans: timestamp, scope, trace ID, and span ID. Logs: every field, including the body and attributes. Metric points: series, timestamps, and value. Two byte-identical log records with the same timestamp therefore collapse into one row. OTLP has no record ID, so this is accepted. |
| `FINAL` | `do_not_merge_across_partitions_select_final=1` is safe because each partition key derives from a timestamp column that is part of the sort key, so duplicates always share a partition. |
| Rollups | The marks view records the minutes that receive spans. The refresh rebuilds whole minutes from `tracepilot_spans FINAL`. Queries read clean minutes from rollups and every other minute from raw spans, so time ranges neither overlap nor leave gaps. The average is `sum(duration) / count`. p95 and p99 merge t-digest states and are approximate. |
| Queue bounds | Slot and byte semaphores admit a request whole or not at all. Events too large for the byte budget are counted as rejected in OTLP partial success. Workers are fixed in number, retries are bounded (3 attempts with jittered backoff), and invalid content is isolated by bisection. |
| Incidents | A transaction-scoped advisory lock per fingerprint, taken in sorted order, plus deterministic incident IDs, prevents duplicates across replicas. Detection uses deterministic event IDs, so replicas repeat work without creating duplicates. |
| Outbox | Deliveries commit with their domain event. Workers claim them with `FOR UPDATE SKIP LOCKED` under a six-minute lease, which exceeds the five-minute request cap. Delivery is at least once. |
| Tenancy | Handlers take the project and environment from the authenticated principal. ClickHouse queries bind every filter as a typed parameter and always include `project_id` and `environment_id`. `DROP PARTITION` accepts only digit partition IDs. |
| Outbound HTTP | Replay and webhook clients resolve the host, refuse private addresses unless allowed, dial the checked address, ignore proxies, and do not follow redirects. |
| Shutdown | OTLP servers stop before the pipelines drain, so no handler waits on a pipeline that has stopped. |

## 4. Fixed findings

### F1. Webhook deliveries collapsed for repeated transitions (P1)

- **Problem:** The delivery ID was a hash of the payload. A manual status change sends
  `{incident_id, environment_id, previous_status, status, actor_user_id}`. A second
  `RESOLVED` by the same operator produced the same ID, and `ON CONFLICT DO NOTHING` discarded it. A
  second system reopen of the same incident had the same problem.
- **Evidence:** `outbox.webhookDeliveryID` (payload digest), `incidents.UpdateIncidentStatus`, and the
  `EnqueueWebhookDelivery` query.
- **Impact:** Subscribers silently missed the second and later notifications until retention removed
  the first delivery (30 days by default).
- **Fix:** `EnqueueDeliveries` now takes the domain event's key, and the delivery ID derives from the
  webhook and that key:
  - an opening uses `opened:<incident ID>`;
  - a reopen uses its status-change ID;
  - a manual transition uses its status-change ID.

  Retried batches still queue nothing new, because their occurrences are filtered before any
  transition happens. The webhook payload is unchanged.
- **Validation:**
  - `outbox_test.go` checks that one event keeps one ID, that different events get different IDs,
    and that a missing key is refused.
  - `TestRepeatedTransitionsEachNotify` (PostgreSQL integration) expects two `incident.resolved`
    deliveries after resolve, open, and resolve. It was written but not run here.
- **Trade-off:** None. Old deliveries keep their old IDs. A retry that spans the deploy cannot repeat
  an opening, because the incident and its occurrence already exist.

### F2. Rollup refresh backlog and visibility (P1)

- **Problem:** Each detection run made one refresh pass of at most 1,000 dirty `(project,
  environment, minute)` rows. Every active environment adds one dirty minute per minute. With more
  than 1,000 active environments, the backlog grew without bound. A rollup failure counted only as a
  detection failure, and the number of minutes refreshed was discarded.
- **Impact:** Results stayed correct, because stale minutes are read from raw spans. Windows of an
  hour or more grew slower, and nothing showed why.
- **Fix:**
  - `refreshRollups` repeats passes while a pass returns a full batch, within the existing 20-second
    deadline.
  - `tracepilot_span_rollup_runs_total{outcome}` and `tracepilot_span_rollup_minutes_total` report
    each run.
- **Validation:** `detection_test.go` checks three things: a 2,500-minute backlog drains in three
  passes, an error stops the loop, and a cancelled context stops the loop.
- **Trade-off:** Throughput is still bounded by the deadline. How many minutes fit in 20 seconds was
  not measured. Watch `span_rollup_minutes_total` against the number of active environments.

### F3. ClickHouse exception code in errors (P2)

- **Problem:** Failures were reported as `ClickHouse query failed (HTTP 500)`. Operators could not
  tell a memory limit from a timeout or a schema problem.
- **Fix:** The error now includes `X-ClickHouse-Exception-Code` when the header is a short number. The
  response body is still never echoed.
- **Validation:** `TestServerFailureNamesOnlyTheExceptionCode`.

## 5. Open findings

| ID | Severity | Finding | Recommendation |
| --- | --- | --- | --- |
| O1 | P1 | Log and trace lists apply a one-hour default only when `from` is absent. An explicit `from` has no maximum. Analysis and performance windows are capped at 30 days. Query gates and `max_execution_time` bound the cost. | Cap list windows at 30 days with a clear 400. This changes the API, so it needs agreement. |
| O2 | P1 | Background workers (detection, rollup refresh, retention, partition maintenance) run on every replica. Results stay correct because writes are idempotent, but the work multiplies with replicas. | See option set B. |
| O3 | P2 | There is no per-project limit on metric series or ingest rate. Attribute count, size, and depth are already limited. | See option set C. |
| O4 | P2 | The metrics pipeline does not retry transient storage errors, so exporters retry. Its workers do not reject queued batches when a drain times out. Shutdown order makes this harmless today. | Mirror the event pipeline's retry and `rejectRemaining` when the pipeline is next changed. |
| O5 | P2 | When a window does not start on a minute boundary, a rollup minute is charted in the bucket that contains its start. Buckets can shift by up to one minute of data. Totals are exact. | Align windows of an hour or more to the minute in the API, or accept and document the shift. |
| O6 | P2 | Rollups expire with raw spans. Rollups cannot be kept longer than raw spans. | Add a separate rollup policy only when a product requirement asks for it. |
| O7 | P2 | Recovery Point Objective and Recovery Time Objective are not defined. The README documents backup and restore commands. | The owner sets both. Schedule restore rehearsals against them. |

### Option set A: durability during a ClickHouse outage

Today an outage turns into `503` with `Retry-After`. Exporters keep the data in their own retry
queues, and no acknowledged data is lost.

1. **Keep synchronous acknowledgment (recommended).** It needs no new infrastructure, and the guarantee
   is simple. Exporter queue sizes decide how long an outage can last without loss.
2. **Local disk spool.** Acknowledge after an fsync to a bounded spool, then replay into ClickHouse.
   Outages are absorbed locally, but the spool needs a volume, replay, corruption handling, and its own
   limits. In return, losing the disk loses data.
3. **External queue (Kafka, NATS).** Gives the strongest decoupling and the highest operational cost.
   Justified only by measured ingest bursts or by many replicas.

### Option set B: background work across replicas

1. **Accept the duplicate work (recommended while there is a single replica).** It is correct today.
2. **Leader lock per worker.** Hold a PostgreSQL advisory lock on a dedicated connection, and run each
   worker only while the lock is held. This is small and uses existing infrastructure. Do it before
   running more than one replica.
3. **Split worker processes.** Run one worker deployment beside API replicas. Do this only when
   measurements show workers competing with ingest.

### Option set C: metric cardinality

1. **Per-project active-series cap in memory (recommended first step).** Count series per project per
   replica, and reject new series past the cap through OTLP partial success with a `cardinality` reason.
   It is cheap, but approximate across replicas.
2. **Cap enforced in ClickHouse.** Count series from `tracepilot_metric_points` periodically, and gate
   ingest on the result. Accurate across replicas, but slow to react.
3. **Per-token ingest rate limits.** These protect throughput, not series count. They complement 1 or 2.

## 6. Files changed

| File | Change |
| --- | --- |
| `backend/internal/storage/postgres/outbox/outbox.go` | Delivery ID from the domain event key |
| `backend/internal/storage/postgres/outbox/outbox_test.go` | New unit tests |
| `backend/internal/storage/postgres/incidents/occurrences.go` | Pass the opening and reopen keys |
| `backend/internal/storage/postgres/incidents/status.go` | Pass the status-change ID |
| `backend/internal/storage/postgres/analytics/incident_analysis.go` | Pass the opening key |
| `backend/internal/storage/postgres/incident_reopen_integration_test.go` | Repeated-transition test |
| `backend/internal/storage/clickhouse/rollups.go` | Export `RollupBatch` |
| `backend/cmd/tracepilot/detection.go` | Drain loop and rollup metric |
| `backend/cmd/tracepilot/detection_test.go` | New unit tests |
| `backend/internal/observability/metrics/metrics.go`, `observers.go` | Rollup metrics |
| `backend/internal/storage/clickhouse/client.go`, `client_test.go` | Exception code in errors |
| `docs/FLOW.md`, `docs/diagrams.html` | Rollup loop and delivery identity |

## 7. Schema, migration, and rollback

- No schema, index, or migration changes.
- Rolling back to the previous image is safe. Deliveries created by this version keep their IDs, and
  the previous version derives IDs only for new events.

## 8. Test results

| Check | Result |
| --- | --- |
| `go build ./...` | Passed |
| `go vet ./...` | Passed |
| `go test ./...` (unit) | Passed |
| `go test -race ./...` | Not run: no C toolchain on the audit machine (`CGO_ENABLED=0`, no gcc) |
| PostgreSQL and ClickHouse integration tests | Not run: the Docker daemon was unavailable. Run with `TRACEPILOT_INTEGRATION_TEST=1` and `TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1` against disposable databases. |
| Benchmarks, `EXPLAIN` | Not run |
| Frontend checks | Not run: no frontend change |

## 9. Deployment checklist

1. Run the PostgreSQL and ClickHouse integration suites against disposable databases, including
   `TestRepeatedTransitionsEachNotify`.
2. Run `go test -race ./...` in CI on a machine with a C toolchain.
3. Add alerts on:
   - `tracepilot_span_rollup_runs_total{outcome="error"}`;
   - `tracepilot_rejected_records_total`;
   - `tracepilot_ingestion_admission_timeouts_total`;
   - `tracepilot_retention_errors_total`.
4. Keep a single backend replica until option set B is decided.
5. Set the exporters' retry queue sizes to cover the longest ClickHouse outage you accept (option
   set A).
6. Define the RPO and RTO, then rehearse the backup and restore steps in the README against them.

## 10. Future scalability (P3, evidence first)

ClickHouse replication, sharding, and multi-region deployment add operational cost that no measured
workload justifies yet. Re-evaluate them when the measurements in the README's capacity section are
exceeded.

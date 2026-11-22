# TracePilot Development Roadmap

TracePilot should qualify its current Go modular monolith and React console for a multi-project pilot. PostgreSQL owns management and incident data. ClickHouse owns telemetry. Ship the two-replica pilot before scaling the processes independently or expanding enterprise administration.

## Current baseline

- Query-time retention covers trace matches and summaries, parent and child spans in analytics, and the raw portion of a minute that crosses the Performance cutoff. Trace detail pages stop at a byte budget and keep cursor ordering.
- The ClickHouse adapter streams metric detection one series at a time and shares JSONEachRow decoding across metrics, logs, retention, and buffered queries. Trace detail reads one wire row at a time to measure the page budget.
- A local 60-second smoke run on three projects accepted about 486 spans/s, 486 logs/s, and 99 metric points/s. All 207 ingest requests and all 300 API reads succeeded. Overall read p95 was 303 ms with a 24-hour read window and about 34,000 prefilled records. This run establishes a reproducible starting workload. [Load generator setup](../README.md#measured-capacity)

## 1. Establish the pilot workload

- [ ] Define expected projects, traffic skew, spans/logs/metric points per second, event size, series cardinality, retention, and console concurrency. Record which read windows the pilot actually needs.
- [ ] Populate a disposable stack with the expected retained record volume and timestamp distribution. Run the 60-minute mixed workload twice, then run a burst where one project consumes most of the offered traffic.
- [ ] Capture offered, persisted, rejected, and retryable counts by signal. Capture per-endpoint p95/p99, queue peak and drain, database connection wait, ClickHouse bytes read, and time from ACK to visible telemetry. Keep build ID, resource limits, and workload parameters alongside the results.
- [ ] Test an interrupted database write and exporter retry. Verify the same event ID is readable once and the corresponding incident occurrence is persisted once.

Initial thresholds to agree with the pilot owner are p95 at or below 1 second for trace and log lists, p95 at or below 2 seconds for 24-hour topology and performance reads, and at least 99.9% successful valid requests in steady state. Count client-side skipped starts, overload rejections, and partial OTLP failures separately so the offered rate is visible. [Load generator](../backend/cmd/loadgen/main.go), [Ingest write contract](../backend/internal/application/logs/persist.go)

## 2. Qualify two backend replicas

| Order | Work | Acceptance test |
| --- | --- | --- |
| 1 | Define the maximum time between a membership or ingest-token revocation and enforcement across replicas. Check the authoritative store for sensitive operations and propagate cache invalidation where needed. | Warm replica B's caches, revoke on A, then read, write, export OTLP, and keep an SSE stream open on B. Enforce the agreed interval. |
| 2 | Add cross-replica Live Tail delivery with bounded subscriber queues. Coordinate detection and rollup refresh so a single replica owns each scheduled pass. | Ingest on A and consume SSE on B. Stop the active detection replica during a pass and verify the next pass runs, without duplicate incident occurrences or webhook records. |
| 3 | Add project-level budgets for ingest and expensive reads alongside the existing process-wide limits. Budget PostgreSQL and ClickHouse connections across the full replica count. | Saturate one project and verify the other projects remain within the agreed latency and error budgets. Drain one replica and verify acknowledged records remain readable. |
| 4 | Support expand-and-contract migrations with a single migration job and an explicit schema compatibility policy. Rehearse startup, readiness, rolling upgrade, and rollback of the application. | Run the previous and new binaries together through a compatible schema upgrade. Restore both databases on a fresh stack and compare telemetry, incidents, and audit state before cutting over. |

The Compose deployment uses static addresses and host ports today. Exercise the pilot through a load balancer and a separate disposable configuration. [Runtime lifecycle](../backend/cmd/tracepilot/serve.go), [Live Tail](../backend/internal/application/live/hub.go), [Schema readiness](../backend/internal/storage/clickhouse/schema.go)

## 3. Reduce the measured bottlenecks

- [ ] Profile trace duration filtering, topology evidence, and Performance top endpoints with realistic history. Reduce scans and intermediate groups first. Evaluate a trace summary or projection if the duration filter still misses its latency budget.
- [ ] Reduce retries shared across projects and measure the cost of a ClickHouse success followed by a PostgreSQL incident-write failure. Define the required durability and delivery semantics before introducing a persistent queue.
- [ ] Profile the console's Overview fan-out. Load the primary data first and handle overload responses without amplifying retry traffic.
- [ ] Compare vertical scaling and two replicas on the same data at equal total CPU and memory. Use sustainable throughput, p95/p99, and database saturation to choose the topology. Split ingest, API, or background workers when the measurements show a need for independent scaling or failure isolation.

Exit this phase when two repeated tests meet the pilot thresholds with the intended retained volume, and the results explain the cost and capacity of the chosen deployment. [Trace queries](../backend/internal/storage/clickhouse/traces.go), [Topology queries](../backend/internal/storage/clickhouse/analytics.go), [Overview queries](../frontend/src/features/overview/use-overview-data.ts)

## 4. Deliver enterprise controls

- [ ] Add OIDC sign-in and group-to-project role mapping. Cover offboarding and emergency administrator access. Add organization-level ownership when the deployment needs to manage multiple independent organizations.
- [ ] Define token rotation and audit export workflows. Test project and environment isolation with regular users, operators, and ingest tokens across each supported signal.
- [ ] Set recovery point and recovery time objectives with the pilot owner. Automate paired database backups, rehearse restore, and document failure triage, rollback, and credential rotation.
- [ ] Continue the onboarding and incident workflow backlog with expiring account recovery, scoped incident rules, evaluation previews, and notification routing.

Release after the pilot passes its load, isolation, rollout, and recovery tests. Preserve the two-database ownership boundaries in [Application structure](STRUCTURE.md) and update the [flow diagrams](diagrams.html) when an application flow changes.

# TracePilot application flow

The [HTML source map](diagrams.html#reading-guide) describes file responsibilities, function signatures,
usage examples, and call chains. Its [file and function index](diagrams.html#source-map) includes search
and filters for tests and generated code.

The corresponding [diagrams](diagrams.html) open in a browser and need an internet connection to load
the rendering library. Numbers in the form `[n]` refer to diagram numbers.

One Go process runs the management API, OTLP receiver, and background workers. PostgreSQL and ClickHouse
run separately. ClickHouse is required for raw logs. The React console runs separately from the backend.

`cmd/tracepilot` wires dependencies and manages the process lifecycle. `application` coordinates use
cases through small interfaces. `domain` contains pure rules. `ingest`, `transport/httpapi`, and
`storage` handle protocol and persistence details. `storage/hybrid` combines the PostgreSQL store with the
ClickHouse log store, and the event writer in `cmd/tracepilot` routes ingested logs through
`application/logs`. `application/session` verifies credentials and
handles password changes, and `application/live` manages the in-memory telemetry hub. In the console,
`shared/session` provides the authentication context and `app` owns the session lifecycle and route
composition. Feature modules stay independent of `app`. See the [MVP scope, release criteria, and
roadmap](../README.md#mvp-scope-and-release-criteria).

## 1. Overview [1]

- Your application sends OTLP data over gRPC (`:4317`) or HTTP (`:4318`) with an **ingest token**.
- The receiver authenticates and normalizes the data, then sends it to a bounded pipeline that writes
  batches by scope. PostgreSQL stores traces and metrics. ClickHouse stores all raw logs.
- When a log triggers an incident, PostgreSQL also stores a compact error envelope. The incident,
  evidence, occurrence, and outbox entries commit in the same PostgreSQL transaction. Acknowledgment
  waits for ClickHouse to flush and for all required PostgreSQL transactions to succeed.
- Background workers detect anomalies every minute, send webhooks, create partitions, and remove expired
  data.
- The **console** calls the REST API at `/api/v1` on `:8080` with an access token.
- The console keeps the project and environment in the URL and sends them in `X-Project-ID` and
  `X-Environment`. Both selectors use the same popover. Use the keyboard or pointer to navigate, then
  confirm the new scope. Esc, Tab, and clicking outside close the menu. The menu stays anchored to the
  button and within the viewport.
- On phones (below 768px) the header is one row with a scope chip that opens the project and
  environment choices in a bottom sheet. A bottom bar links Overview, Traces, Logs, and Incidents, and
  its More sheet holds the other sections, the theme, and sign-out. Sheets trap focus and close on Esc
  or the backdrop. Tables become cards, log rows stack their fields, and the log explorer keeps search
  inline while service, trace, and span filters move into a sheet. Phone-only controls render only at
  that width, so screen readers and tests never meet two copies of the navigation.
- Changing scope clears query caches, remounts the page, and discards the previous Live Tail buffer.
  Page links retain the scope and time range. A drill-down with explicit `from/to` keeps that absolute
  range through search and browser Back navigation.

## 2. Startup and shutdown [2]

### Container deployment

`deploy/compose.yaml` runs the application against external PostgreSQL and ClickHouse services. Compose
reads `backend/.env` and `frontend/.env` using paths relative to `deploy/`. Create `certs/` in advance
and use it for public CA certificates. The `migrate` service runs `migrate all` with the backend image
and its health check disabled. Exit code 0 allows the backend to start. The console starts when the
backend is healthy. `up --wait` waits for these conditions before returning successfully.

The stack enforces production validation and disables both auto-migration flags for `serve`, leaving
schema updates to the migration service. Container and network names are scoped to the Compose project,
and service DNS names stay stable. Ports bind to loopback and host nginx terminates HTTPS. The
filesystem is read-only, with tmpfs for rendered nginx configuration and temporary files. Containers
have a 60-second stop grace period.

For an upgrade, pause exporters, stop the backend and let it drain, back up both databases, then run the
new image's migrations before starting the application. Readiness and retries handle database failures
after the initial startup checks.

### Backend lifecycle

Before starting a new version, run `migrate all`. It upgrades PostgreSQL under its own advisory lock,
upgrades ClickHouse using its own journal, then verifies readiness of both databases. A failed step
prevents the application from starting. Previously completed steps remain applied because the databases
have separate transactions. `make start` and `make dev` run the batch before `serve`, including when
Make runs in parallel.

`serve` reads two independent flags from `backend/.env`. `TRACEPILOT_DATABASE_AUTO_MIGRATE` applies to
PostgreSQL and `TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE` applies to ClickHouse. Both default to false. An
enabled flag runs that database's migrations before starting listeners or workers. A migration error
stops startup. Runtime requires both databases. Raw log storage depends on ClickHouse.

PostgreSQL limits lock waits to `DATABASE_MIGRATION_LOCK_TIMEOUT` (10 seconds) and each migration
statement to `DATABASE_MIGRATION_STATEMENT_TIMEOUT` (5 minutes). The migration connection uses
`DATABASE_CONNECT_TIMEOUT` by default, with `connect_timeout` in the URL taking precedence. ClickHouse
requests use their configured timeout. PostgreSQL cancellation takes effect between files and an active
statement may run until its timeout.

Individual CLI entry points, `migrate_postgres.go` and `migrate_clickhouse.go`, validate their own
commands, load the settings for their respective database, and enforce its production TLS policy. These
commands use `backend/.env` and can run independently of the other database or application settings.
They call `storage/postgres/migrations.RunWithConfig` and `storage/clickhouse.RunMigration`. Startup
auto-migration in `storage/hybrid.Open` calls the same runner when that database's flag is enabled.

`storage/postgres/migrations.Run` holds the advisory lock for every `up` and `down` operation, including
creation of the journal. Callers rely on that lock. `status` and `version` are read-only and skip the
mutation lock. The CLI shares helpers for configuration, shutdown signals, and logging. The status
helper computes the target and pending versions from the selected history. Each storage adapter owns its
migration state and versions, and architecture tests enforce their independence. Cross-database data
transfers are outside the migration CLI.

`migrate all` calls `storage/hybrid.MigrateAll`, which runs the two independent managers in sequence. It uses the runtime client and readiness
code paths and requires only database credentials. All commands log JSON through `log/slog` with
`service.name` and `service.version`. `serve` applies `LOG_LEVEL`. The `config` package returns errors
for the calling command to log once.

The ClickHouse runner validates the engine, partition key, deduplication key, and columns before
adopting a legacy table. Version `000002` removes the global log TTL when present and verifies the
result before marking the version clean. Readiness requires project-scoped retention without a table TTL.

The binary embeds SQL from `backend/db/postgres/migrations` and `backend/db/clickhouse/migrations`. Each
database derives its target version from its own migration files and history. The PostgreSQL baseline
`000001_init.up.sql` creates tables, constraints, indexes, partitions, and functions in a single
transaction. `000002_drop_postgres_logs` drops the legacy `logs` table. Its down migration recreates an
empty table. `000003_project_retention` adds the optional project override. The baseline has no down
SQL, so a baseline rollback is rejected before journal or table changes. Add changes under a new ID
and leave applied files intact.

`migrate postgres status` and `migrate clickhouse status` read the current version, target, dirty state,
and pending versions without creating metadata. PostgreSQL uses `schema_migrations`. The ClickHouse
journal, `tracepilot_schema_migrations`, records a dirty entry before DDL and a clean entry after
success. A clean entry takes precedence for the same version. A failure or disconnect can leave a dirty
entry. Inspect and repair the schema before retrying. Dirty-state recovery is a manual operation.

The first ClickHouse migration adopts an existing log table after validating its layout,
preserving rows. Individual commands need only their database, while `all` requires both. Coordinate a
single updater for non-idempotent ClickHouse DDL, which runs without a migration lock. The consolidated
PostgreSQL baseline targets fresh databases. Older histories require a reviewed transition or data
transfer. Keep the published history append-only. Embedded SQL removes the need for a runtime SQL path
setting.

The database-first workflow is to add a migration, update named queries in `backend/db/queries`,
regenerate sqlc from the same migration history, update storage code, and run integration tests against
a disposable database. `db/external` describes migration-tool metadata for sqlc. PostgreSQL enforces
incident status and severity as well as nonempty, one-dimensional webhook subscriptions with supported
events. The constraints also apply to imports and direct SQL writes. A constraint migration rejects
invalid existing rows and leaves any data repair or dirty-journal recovery to the operator.

The server starts in this order:

1. Load defaults, then `backend/.env`, then environment variables. Invalid configuration returns exit
   code 2. Other startup failures return exit code 1.
2. If `TRACEPILOT_AUTH_JWT_SECRET` is missing, generate a random development key and warn. Production
   validation requires a configured key in step 1.
3. Run enabled auto-migrations, PostgreSQL first and ClickHouse second. Open one shared PostgreSQL pool
   and one shared ClickHouse HTTP client. The PostgreSQL pool defaults to 2–20 connections and a
   5-second connect timeout. A URL `connect_timeout` takes precedence. Connection lifetimes have 10%
   jitter to stagger replacement. The minimum connection budget accounts for event and metric workers,
   plus two connections. Pooled sessions set `jit=off`, because LLVM compilation cost more than the
   short span aggregates it would speed up. A URL `jit` parameter takes precedence.
4. Check both databases concurrently under the longer of `DATABASE_CONNECT_TIMEOUT` and
   `CLICKHOUSE_REQUEST_TIMEOUT` (10 seconds by default). PostgreSQL checks the embedded target version
   and clean state through the runtime pool. ClickHouse checks the version, dirty state, schema, TTL,
   and read access. A failure cancels the other check and closes both clients. Databases with
   auto-migration disabled must be migrated in advance with an individual command or `migrate all`.
   Next, prepare PostgreSQL partitions. Projects and environments are created through the console or
   API. Readiness checks both stores concurrently under a shared 2-second deadline. The PostgreSQL
   version query also verifies connectivity. A ClickHouse outage causes readiness to fail.
5. Set up replay if enabled, metrics, the events and metrics pipelines, the OTLP receiver, and the API.
6. Run the API and OTLP receiver concurrently. The process stops if either core component stops or
   fails. Detection, partition maintenance, optional webhook delivery, and configured retention workers
   restart with backoff on failure.

Shutdown begins on SIGINT, SIGTERM, or failure of a core component. Listeners stop accepting traffic and
wait for requests. The events and metrics pipelines then drain concurrently within `shutdown_timeout`
(15 seconds), followed by the ClickHouse HTTP client and PostgreSQL pool closing. Allow a container stop
grace period of at least 35 seconds. Exporters retry unacknowledged data.

## 3. Registration, sign-in, and refresh [3]

- For a fresh production installation, run `create-admin --email ... --name ... --password-stdin` after
  migration. The command connects only to PostgreSQL, checks production TLS and schema readiness,
  applies the same password rules as the API, and creates the account and audit entry in one
  transaction. It reads a single password line from stdin and excludes it from arguments and logs. An
  existing email causes the command to fail and leaves that account intact.
- **Registration.** The development command `tracepilot seed` creates only the super admin account
  `admin@tracepilot.local` with password `admin-password-123`. It uses the same account creation path
  as `create-admin`, so it requires a migrated schema and records an audit entry. Reruns leave an
  existing account with that email unchanged. Create the project and environment in the console after signing in. When
  `ALLOW_REGISTRATION=true`, the form and API create regular accounts. A super admin can grant
  `platform_admin` in **Settings → Accounts** or add an account to a project in **Settings → Members**.
  Passwords must be 12–256 characters. Setting `ALLOW_REGISTRATION=false` closes public registration.
- **Sign-in.** Limits apply per IP and allow ten attempts per email during the configured window.
  Unknown emails and wrong passwords get the same message and take the same time to check. On success,
  the server creates a refresh session, sets the `tp_refresh` cookie (`HttpOnly`, `SameSite=Strict`,
  path `/api/v1/auth`), and returns the project list and an **access token** (HS256 JWT, valid for 15
  minutes).
- The HTTP handler decodes the request, applies rate limits, and calls
  `application/session.Authenticate` through a credential reader interface. The use case normalizes the
  email, verifies the password, and rejects locked accounts. It returns database errors separately from
  credential failures. The HTTP layer owns JWT issuance, cookies, and the response contract.
- The browser keeps the **access token** in memory. When the API returns `TOKEN_EXPIRED`, the console
  calls `/auth/refresh` with `X-TracePilot-Refresh` and retries the original request. Refresh rotates
  the token and rejects reuse of the old token. Tabs share the cookie, so they refresh one at a time
  under the `tracepilot-refresh` Web Lock and each sends the cookie set by the previous refresh. A
  failed refresh returns 401 without clearing the cookie, because a concurrent request may already have
  replaced it. Only sign-out clears it. Sessions expire after seven days of inactivity or 30 days total.
- **Password changes and account locks.** The transaction advances `tokens_valid_after` by at least one
  microsecond. The access JWT carries this value in `tp_ver`, and REST and SSE require an exact match.
  Revocation works even if issuance and revocation occur in the same second or the application clock
  runs slow. Refresh and sign-in read the current version. Legacy JWTs without `tp_ver` remain valid
  only for accounts that have never had tokens revoked. Accounts created by an admin with a temporary
  password can call only `/auth/me` and `/auth/password` until changing it.
- `application/session.ChangePassword` verifies the current password, rejects reuse, and calls storage
  to change the password and revoke sessions and tokens in one transaction. It preserves the current
  browser session if a refresh cookie is present. The handler reloads the account and returns the
  session view and a new JWT.
- The JWT issued after a password change uses the same configured issuer, audience, and TTL as sign-in
  and refresh.

## 4. API authentication and authorization [4]

In order, the request passes through request ID, security headers, metrics, rate limits for `query`,
`auth`, and `replay`, access logging, CORS, panic recovery, body limits, and the router.

The router enforces the **scope** declared in the route table.

1. Missing or invalid tokens return 401. Expired tokens return `TOKEN_EXPIRED`.
2. The router reads the account from the database. A missing or locked account or a token issued before
   `tokens_valid_after` returns 401.
3. A temporary password on a route that requires a password change returns 403
   `PASSWORD_CHANGE_REQUIRED`.
4. The `session` scope is available to signed-in users. The `platform` scope requires a super admin.
5. Project routes take the project ID or slug from the path or `X-Project-ID` and cache it in memory for
   30 seconds. Missing and inaccessible projects both return 403 to prevent enumerating names.
6. A super admin is an owner on every project. Other roles come from `project_members`, with a 30-second
   memory cache invalidated on membership changes. The `user` role grants `read, write`. The `owner`
   role also grants `replay, admin`. Insufficient scope returns 403.
7. Resolve the environment from `X-Environment`, defaulting to the first environment and caching for 30
   seconds. An invalid environment returns 400.
8. Wait up to 500 ms for a query slot, then run the handler. The 20-second deadline starts before
   step 2, so account and environment lookups that wait for a pooled connection also end with 504.
   An exhausted slot wait returns 503. An exceeded query deadline returns 504. Public authentication routes under `/auth/*` use
   16 dedicated slots, so telemetry queries cannot exhaust their budget. Prometheus reports wait times
   and active slots in `tracepilot_concurrency_gate_*` and authentication cache activity in
   `tracepilot_auth_cache_requests_total`.

## 5. OTLP ingestion [5]

1. Accept an HTTP or gRPC request. Read `Authorization: Bearer <ingest token>` and `X-Environment` (gRPC
   metadata `x-environment`).
2. Check the authentication failure tracker. A client past `AUTH_FAILURE_LIMIT` for the window receives
   429 before any database lookup.
3. Verify the token signature, issuer, audience, `ingest` type, and expiration. Look up `ingest_tokens`
   through a 30-second memory cache that is invalidated on revocation. An invalid token returns 401. A
   database error returns 503 so the exporter can retry.
4. Resolve the project and environment through `scopeCatalog`, cached for 30 seconds. An unknown
   environment is rejected.
5. gRPC allows up to `MAX_CONCURRENT_REQUESTS` streams per connection. Additional streams wait on the
   client. It uses a 10-second handshake, keepalive, and a 30-minute connection lifetime to rebalance
   across replicas. Acquire a processing slot or return 503, then decode within the compressed and
   decompressed body limits. Normalize records, redact sensitive keys, and scrub secret patterns from
   free text. Count invalid records as `rejected` and continue processing the rest.
6. Reserve slots and bytes for the whole request before putting events in the pipeline. If space stays
   unavailable during `ENQUEUE_TIMEOUT`, return 503 with `Retry-After`. Rejecting before any event is
   written lets exporters retry safely. Requests larger than the whole queue are admitted in parts.
   Capacity is `QUEUE_SIZE` + `WORKERS` × `BATCH_SIZE`. Slots remain reserved until events are
   acknowledged.
7. Workers batch events by scope. `application/logs.Persist` writes raw logs over HTTP as gzipped
   JSONEachRow with `async_insert=1` and `wait_for_async_insert=1`. It then calls `StoreEvents` for
   traces and `StoreLogIncidents` for log errors. The latter stores only a compact error envelope with
   empty attributes, excluding the raw body and resource and scope attributes. It calls
   `RecordOccurrences` and writes the outbox in the same PostgreSQL transaction. PostgreSQL writes for
   logs are limited to incident handling. Spans still mark their minute dirty in the same transaction.
   Metrics use a separate PostgreSQL pipeline.
8. Acknowledge the batch after all steps succeed. Because the databases use separate transactions, a
   later failure can leave data written by an earlier step. The exporter retries with the same IDs.
   ClickHouse deduplicates with `ReplacingMergeTree` and `FINAL` reads, and PostgreSQL keeps its
   idempotent writes. A transient storage failure returns 503 or Unavailable. The worker bisects batches
   with unstorable content to isolate the invalid record, commits the rest, and returns partial success.
   Each event batch gets up to three write attempts, with backoff only between attempts. Cancellation
   during backoff stops further storage calls. Retry metrics count retries that actually start.

`ListLogs`, `GetLog`, and log counts in `TelemetryQuality` read directly from ClickHouse. They preserve
the project, environment, filters, cursor, and trace/span correlation. Log pagination uses ClickHouse
alone. An outage returns 503 for log ingestion and an error for log reads. `Database.StoreEvents` treats
an unexpected log event as a storage error, bypassing the batch split used to isolate invalid content.
Before upgrading an older installation that still has PostgreSQL logs, copy them with the previous
release's `migrate clickhouse backfill` command. Migration `000002` drops the `logs` table.

## 6. Incident lifecycle [6]

- Each direct error records an occurrence. The **fingerprint** combines service, operation, error type,
  HTTP status, and a normalized message. Normalization replaces UUIDs, timestamps, emails, IP addresses,
  and numbers with placeholders. Matching fingerprints are grouped into the same incident.
- Incident statuses are `OPEN`, `INVESTIGATING`, `RESOLVED`, and `IGNORED`. A user with the `write`
  scope can change status. Each change records the actor in the history.
- A `RESOLVED` incident reopens if an occurrence newer than the resolution arrives within the one-hour
  deduplication window. The actor is `system` and the webhook event is `incident.opened` with `reopened:
  true`. An `IGNORED` incident stays ignored when new occurrences arrive.
- The console loads an incident with two queries, one for incident details and one for analysis. The
  header shows status, severity, service, occurrence count, and first and last seen timestamps.
  Investigation, Occurrences, Linked telemetry, and Status history support keyboard navigation and store
  the selected tab in the URL. The three data tabs query the API when opened. Cursor pagination fetches
  25 records at a time, renders the current page, and caches up to 20 loaded pages.
- Analysis shows up to three hypotheses, eight items of evidence, and eight relationships per page.
  Scores are heuristics for ranking possible causes. Selecting a hypothesis filters evidence by ID.
  Original values, series, IDs, and full trace links are available in the details. A successful status
  change updates the header and refreshes history. A failed change leaves the previous status and keeps
  all tabs usable.

## 7. Anomaly detection and root cause analysis [7]

Every minute, a worker pool processes project and environment pairs concurrently with up to four
goroutines. It compares the most recent five minutes with the previous 30-minute baseline. SQL supplies
the signals and a pure domain policy decides whether they are anomalous. Latency needs at least ten
calls in each window. Throughput needs at least 30 baseline calls. An anomaly creates or updates an
incident with evidence. The incident analysis uses evidence and dependency relationships to rank
hypotheses by confidence.

Before detection runs, the existing worker refreshes up to 64 dirty minutes per scope in
`span_minute_rollups` under a separate five-second deadline. It locks the selected minutes with `FOR
UPDATE SKIP LOCKED`, reads a fresh snapshot, replaces each full summary, and marks it clean in the same
transaction. Late spans and retries mark the minute dirty again. Replacing the summary avoids
double-counting. If refresh fails, detection tries the raw query within its remaining budget.

`RequestPerformance`, `DetectServiceEvidence`, `DetectDependencyEvidence`, and the domain policies keep
their contracts. Detection combines counts and total duration from clean minutes and reads raw spans for
dirty, unaggregated, or partially overlapping minutes. Performance uses prepared arrays of timestamps,
durations, and error statuses from summaries. It computes percentiles over individual requests and
avoids averaging per-minute p95 or p99 values. Arbitrary windows, chart buckets, and trace examples used
as evidence keep the same behavior. OTLP metric and counter reset handling, topology, and the trace and
log explorers use their existing functions.

Topology looks for parent spans within the same project and environment and as far back as one hour
before the child window. SQL uses `LATERAL` and `OFFSET 0` to look up a parent by trace and span ID.
Trace listing also looks up spans by individual trace ID. These index lookups avoid scanning span pairs
repeatedly when a new project is absent from PostgreSQL statistics. The integration test runs `ANALYZE`
on the child partition for an existing scope, adds 16,000 spans to a new scope, and checks that topology
and trace listing each meet a five-second query budget.

The baseline migration lists minutes to process. The worker gradually builds the latency arrays. Until
rollups catch up, queries read spans from the same snapshot as the summaries. This lets queries run
during backfill using the existing configuration and process.

## 8. Ingest tokens [8]

A project owner creates tokens with names unique within the project. Tokens expire after one year by
default and can last up to three years. The JWT is displayed **once**. The database stores only its
metadata. Tokens use the separate `INGEST_JWT_SECRET` signing key. Production requires that key to
differ from `JWT_SECRET`. In development, a missing ingest key falls back to `JWT_SECRET` and logs a
warning.

The receiver looks up token metadata in a cache with a configured TTL. When the owner revokes a token
through the API, the replica handling the request clears its local entry and the shared cache entry.
Other replicas may retain local entries until TTL expiry. Separate signing keys let operators rotate
user account keys without interrupting OTLP exporters.

## 9. Webhooks and the outbox [9]

Opening or reopening an incident queues a delivery in the outbox **in the same transaction** as
ingestion. The worker claims deliveries with `FOR UPDATE SKIP LOCKED`. The six-minute lease allows
another attempt after a worker crash. The worker checks the destination URL, requires HTTPS, blocks
private and metadata address ranges, skips redirects, and signs the request with HMAC. Failed attempts
follow the retry policy until delivery succeeds or reaches `failed`. Consumers should expect
at-least-once delivery.

Each poll claims up to `NOTIFICATIONS_WORKERS` deliveries and waits for the entire batch to send and
record its results before polling again. A slow endpoint therefore cannot cause overlapping polls to
accumulate workers or leases.

## 10. Partitioning and retention [10]

PostgreSQL telemetry tables are partitioned by UTC day. Startup creates a window of
`DATABASE_PARTITION_WINDOW_DAYS` days starting yesterday (10 by default). Maintenance runs every
`DATABASE_PARTITION_CHECK_INTERVAL` (one hour by default).

In **Settings → Projects → Data retention**, project owners can enter whole days or inherit
`TRACEPILOT_RETENTION_MAX_AGE` (seven days by default, or `0s` for indefinite retention). The policy
covers logs, traces, and metrics across every environment of the project. `GET` and `PUT`
`/api/v1/projects/{projectID}/retention` read and update it. Send `retention_days: null` to restore the
default. The write and its `project.retention_updated` audit event commit together.

The worker reads the policies at startup and every `RETENTION_CHECK_INTERVAL` (one hour by default).
It drops a daily partition only when the entire day has expired for every project. In shared and
default partitions it deletes expired rows in project-scoped batches, locking the policy during each
batch. Span batches invalidate affected rollup minutes in the same transaction. Queries read surviving
spans directly until ingestion schedules those minutes for aggregation again.

Closed incidents and their history default to 90 days, audit entries to 180 days, and completed webhook
deliveries to 30 days. These policies continue to use their own global settings. Open incidents and
in-progress deliveries are excluded.

ClickHouse partitions raw logs according to `TRACEPILOT_CLICKHOUSE_LOG_PARTITION`. Choose day
(`toYYYYMMDD`), month (`toYYYYMM`), year (`toYear`), or the default week (`toStartOfWeek`).
`ReplacingMergeTree` with `FINAL` reads deduplicates exporter retries by scope, service, millisecond
timestamp, and event ID. The ID hash includes the full nanosecond timestamp, so otherwise identical logs
in the same millisecond can have distinct IDs. Stored timestamps and cursors retain nanosecond
precision.

Log reads enforce the current project cutoff immediately. The worker checks for expired rows and
submits a project-scoped ClickHouse `ALTER … DELETE` mutation when needed. The runtime user needs
`ALTER DELETE` on `tracepilot_logs`. Mutation completion and inactive-part cleanup determine when disk
space is reclaimed. Shortening a policy can delete previously stored data at the next sweep.
The partition setting chooses the key for new log tables. Changing an existing key requires a
separate table and data migration.

The log explorer and Live Tail filter the same severity groups (TRACE, DEBUG, INFO, WARN, ERROR, and
FATAL). OTLP numbers 1–24 take precedence, with four levels per group. An unspecified number falls back
to case-insensitive text, including WARNING, INFORMATION, and suffixes 1–4. Blank text defaults to INFO.
Custom text can also be filtered case-insensitively. Storage and API responses preserve the exporter
text. SSE also includes `severity_number`.

## 11. Live telemetry and Trace Waterfall

- **Live streaming.** `GET /api/v1/live/stream` (scope `read`) opens a Server-Sent Events (SSE) stream
  for new logs and incidents after all persistence steps for the batch succeed. The pipeline's
  post-persistence callback publishes through the in-memory `application/live.Hub`, which is independent
  of HTTP and storage. The HTTP adapter subscribes, serializes events as JSON, and closes the connection
  when the hub stops. Streams use a separate limit of 256 concurrent connections and a five-second write
  deadline per frame. They bypass the bounded query gate (`admitQuery`).
- **Stream authorization and recovery.** The stream ends when the JWT expires. Every 15 seconds, it
  checks the account and membership directly in PostgreSQL with a two-second deadline and bypasses the
  role cache. A failed check or revoked access closes the stream. The console shares its API client
  between JSON and SSE, including the access JWT, API origin, project and environment scope, and token
  refresh. It displays 4xx access errors and retries transient connection failures with backoff up to 15
  seconds. Live Tail keeps up to 100 recent events, applies service, severity, body, trace, and span
  filters, and deduplicates against historical results.
- **Trace Waterfall.** The dependency-free `useVirtualList` renders only spans in and around the visible
  scroll area. Even a trace with thousands of spans starts with a small DOM. Rows have a fixed height.
  The detail inspector sits outside the list, so long attributes and stack traces stay clear of other
  rows. The console loads up to 5,000 spans per trace and indicates truncation. Closing the inspector
  returns focus to the selected span or to span search if that row is no longer mounted.
- **Trace investigation.** The header shows the operation, status, and four metrics. The request flow
  summarizes self-time by service and opens the service map on demand. The operation, service, and span
  ID search and the error filter cover all loaded spans, including collapsed branches. Selecting a span
  opens the inspector on the right on wide screens and in a bottom sheet on narrower screens. Use clicks,
  arrow keys, Home, or End to switch between Attributes, Resource, Events, and Raw. Attribute values
  use the full inspector width. Related logs span the full card below the waterfall and inspector,
  with full contents and copy controls on each row. Local log filters search the loaded
  logs. The Log explorer queries the server while retaining the trace or span filter and the time range.
- **Explorer and Overview.** Cursor-based tables fetch more on request, up to 20 pages per query, then
  ask the user to narrow the filters. Timestamps show millisecond precision in the local time zone.
  Overview labels counts from the loaded page and gets aggregate error signals from Performance for the
  same window. It avoids inferring overall system health from a partial incident page or an empty
  traffic result.

## 12. Known limits

- Rate limits apply per process. Use additional limits at the reverse proxy for multiple replicas.
- Live Tail is best-effort and uses memory in the process serving the connection. Events stay within
  that replica.
- The queue byte budget estimates queued data. gRPC decodes before queuing and its stream limit applies
  per connection, so process RSS can exceed the queue budget.
- SSO, MFA, email password reset, and project or environment deletion are outside the current feature
  set.
- See [measured capacity](../README.md#measured-capacity) for load results and deployment limits.
  `ListTraces` picks the page from matching spans first, ordered by each trace's latest matching span,
  then summarizes only those traces. Matching still reads every span in the window, so cost grows with
  the window's span count. A duration filter needs whole traces and summarizes every match first.
  Requests without `from` or a `traceId` filter read only the hour before `to`, the page cursor, or the
  current time. The console checks its connection through `GET /projects/{id}`. The November 16, 2026
  measurement removed 504 timeouts from repeated span lookups. At 1,000 records and five reads per
  second, PostgreSQL with 2 vCPU still saturated and returned 503 under backpressure. With 4 vCPU, the
  same code and workload completed a 60-second run without failures and with read p95 of 689 ms. Use
  these short measurements to choose a test setup. Sustained load and a full one-hour query window still
  need measurement.
- A ClickHouse outage returns 503 from readiness on all replicas while sign-in remains available. In
  testing, readiness took about 30 seconds to recover after ClickHouse returned.
- PostgreSQL integration tests require a disposable database (`TRACEPILOT_INTEGRATION_TEST=1`).
  ClickHouse integration tests require a disposable instance
  (`TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1`, `TRACEPILOT_CLICKHOUSE_TEST_URL`).
- The migration histories were separated before release and target fresh databases. Upgrading an earlier
  history requires a reviewed transition. Existing migration versions on real databases stay in place.

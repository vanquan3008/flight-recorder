# Configuration

The backend reads all of its settings from environment variables. This page explains where they are
loaded from, which source wins when the same setting appears more than once, and how variable names
map to settings.

## Sources

The backend reads at most one environment file:

- **From the repository root**, it reads `backend/.env`.
- **From the `backend` directory**, it reads `.env`.

The directory must contain `go.mod`. In any other working directory, the backend reads no file.

| Case | Behavior |
| --- | --- |
| The file is missing. | Allowed. Defaults and process variables apply. |
| The file is malformed. | Startup stops. The error names the file but does not show its contents. |

The file never changes the process environment.

## Precedence

Each setting is resolved on its own. The first source that defines it wins:

1. Process environment.
2. Environment file.
3. Built-in default.

A variable that is set counts as configured, even when its value is `false`, `0`, `0s`, or empty. An
empty value clears a string or a list. For a number, boolean, or duration, an empty value is an error.
Setting one variable changes only that setting. For example, `TRACEPILOT_QUEUE_WORKERS=16` leaves the
queue size, batch size, and timeouts at their defaults.

## Names and types

A variable name is `TRACEPILOT_` followed by the setting's path in upper case, joined with
underscores. The backend reads no other names. For example, `storage.retention.max_age` becomes `TRACEPILOT_STORAGE_RETENTION_MAX_AGE`.

| Type | Format |
| --- | --- |
| Duration | Go duration syntax: `250ms`, `30s`, `1h`, `168h` |
| Boolean | `true`, `false`, `1`, `0` |
| Integer | Base 10 |
| Byte size | Integer number of bytes |
| List | Comma-separated values; blank entries are ignored |
| Log level | `debug`, `info`, `warn`, or `error`, in any case |

An invalid value stops startup. The error names the setting and the variable that supplied it, but
not the value, so secrets do not end up in logs.

## Groups

| Group | Prefix | Owns |
| --- | --- | --- |
| `app` | `TRACEPILOT_APP_` | Production mode, shutdown deadline, and listener TLS |
| `http` | `TRACEPILOT_HTTP_` | API listener, CORS, trusted proxies, rate limits, query limits, and timeouts |
| `database` | `TRACEPILOT_DATABASE_` | PostgreSQL URL, pool, session timeouts, migrations, and partitions |
| `clickhouse` | `TRACEPILOT_CLICKHOUSE_` | ClickHouse endpoint, credentials, timeouts, and log partitioning |
| `auth` | `TRACEPILOT_AUTH_` | JWT secrets, token lifetimes, and cookies |
| `cache` | `TRACEPILOT_CACHE_` | Cache driver, Redis connection, TTLs, and capacities |
| `storage` | `TRACEPILOT_STORAGE_RETENTION_` | Retention ages, check interval, and delete batch size |
| `queue` | `TRACEPILOT_QUEUE_` | Queue capacity, byte budget, batch size, flush interval, timeouts, and workers for both pipelines |
| `ingestion` | `TRACEPILOT_INGESTION_` | OTLP listeners, default scope, payload and attribute limits, redaction, and authentication throttling |
| `observability` | `TRACEPILOT_OBSERVABILITY_` | Log level and log format |
| `replay` | `TRACEPILOT_REPLAY_` | Request replay switch, allowed hosts, and limits |
| `notifications` | `TRACEPILOT_NOTIFICATIONS_` | Webhook delivery switch, encryption key, polling, workers, and request timeout |

[`backend/.env.example`](../backend/.env.example) lists every setting with its default, grouped as
above. Defaults are defined once, in `backend/internal/config/defaults.go`, and a test fails when the
template misses a setting or names one that does not exist.

## Upgrading configuration names

Update stored environment files and process variables before upgrading if they use the previous names.
Copy values to the current names to preserve production checks, listener TLS, retention, and worker settings.

| Previous name or prefix | Current name or prefix |
| --- | --- |
| `TRACEPILOT_PRODUCTION` | `TRACEPILOT_APP_PRODUCTION` |
| `TRACEPILOT_SHUTDOWN_TIMEOUT` | `TRACEPILOT_APP_SHUTDOWN_TIMEOUT` |
| `TRACEPILOT_TLS_` | `TRACEPILOT_APP_TLS_` |
| `TRACEPILOT_LOG_LEVEL` | `TRACEPILOT_OBSERVABILITY_LOG_LEVEL` |
| `TRACEPILOT_RETENTION_` | `TRACEPILOT_STORAGE_RETENTION_` |
| Queue and batch settings under `TRACEPILOT_INGESTION_` | Corresponding `TRACEPILOT_QUEUE_` settings |
| `TRACEPILOT_APP_REPLAY_` | `TRACEPILOT_REPLAY_` |
| `TRACEPILOT_APP_NOTIFICATIONS_ENABLED` | `TRACEPILOT_NOTIFICATIONS_ENABLED` |
| `TRACEPILOT_WEBHOOK_` | `TRACEPILOT_NOTIFICATIONS_` |

## Production requirements

With `TRACEPILOT_APP_PRODUCTION=true`, startup fails unless every item below holds:

- `TRACEPILOT_AUTH_JWT_SECRET` and `TRACEPILOT_AUTH_INGEST_JWT_SECRET` are set and different.
- Each secret has at least 32 characters and is not a sample value from the repository.
- `TRACEPILOT_HTTP_METRICS_TOKEN` is not a sample value.
- The PostgreSQL URL uses `sslmode=require`, `verify-ca`, or `verify-full`, unless it points to a Unix
  socket.
- The ClickHouse URL uses HTTPS.

`migrate` loads `app.production` and the selected storage groups. `create-admin` loads
`app.production` and the `database` group. Settings outside these scopes do not block the commands.

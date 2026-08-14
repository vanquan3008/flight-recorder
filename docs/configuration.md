# Configuration

Flight configuration format version is `1`. Precedence is CLI `--config` path, environment overrides, YAML values, then safe defaults. An explicitly selected missing file, unknown YAML field, second YAML document, file larger than 1 MiB, invalid duration, or invalid limit fails startup before listening.

Use [configs/flight.example.yaml](../configs/flight.example.yaml) as the reference. Production secrets should come from environment injection or a secret-mounted mechanism; never commit a populated database URL.

## Environment overrides

| Variable | YAML field |
|---|---|
| `FLIGHT_CONFIG_FILE` | Selects the YAML file when `--config` is absent |
| `FLIGHT_HTTP_ADDRESS` | `http.address` |
| `FLIGHT_HTTP_READ_HEADER_TIMEOUT` | `http.read_header_timeout` |
| `FLIGHT_HTTP_READ_TIMEOUT` | `http.read_timeout` |
| `FLIGHT_HTTP_WRITE_TIMEOUT` | `http.write_timeout` |
| `FLIGHT_HTTP_IDLE_TIMEOUT` | `http.idle_timeout` |
| `FLIGHT_HTTP_MAX_HEADER_BYTES` | `http.max_header_bytes` |
| `FLIGHT_HTTP_MANAGEMENT_MAX_BODY_BYTES` | `http.management_max_body_bytes` |
| `FLIGHT_INGESTION_GRPC_ADDRESS` | `ingestion.grpc_address` |
| `FLIGHT_INGESTION_HTTP_ADDRESS` | `ingestion.http_address` |
| `FLIGHT_INGESTION_PROJECT_ID` | `ingestion.project_id` |
| `FLIGHT_INGESTION_PROJECT_SLUG` | `ingestion.project_slug` |
| `FLIGHT_INGESTION_PROJECT_NAME` | `ingestion.project_name` |
| `FLIGHT_INGESTION_ENVIRONMENT_ID` | `ingestion.environment_id` |
| `FLIGHT_INGESTION_ENVIRONMENT_NAME` | `ingestion.environment_name` |
| `FLIGHT_INGESTION_MAX_REQUEST_BYTES` | `ingestion.max_request_bytes` |
| `FLIGHT_INGESTION_MAX_RECORDS_PER_REQUEST` | `ingestion.max_records_per_request` |
| `FLIGHT_INGESTION_MAX_ATTRIBUTES_PER_RECORD` | `ingestion.max_attributes_per_record` |
| `FLIGHT_INGESTION_MAX_ATTRIBUTE_VALUE_BYTES` | `ingestion.max_attribute_value_bytes` |
| `FLIGHT_INGESTION_MAX_ATTRIBUTE_DEPTH` | `ingestion.max_attribute_depth` |
| `FLIGHT_INGESTION_QUEUE_SIZE` | `ingestion.queue_size` |
| `FLIGHT_INGESTION_QUEUE_MAX_BYTES` | `ingestion.queue_max_bytes` |
| `FLIGHT_INGESTION_ENQUEUE_TIMEOUT` | `ingestion.enqueue_timeout` |
| `FLIGHT_INGESTION_BATCH_SIZE` | `ingestion.batch_size` |
| `FLIGHT_INGESTION_BATCH_TIMEOUT` | `ingestion.batch_timeout` |
| `FLIGHT_INGESTION_FLUSH_INTERVAL` | `ingestion.flush_interval` |
| `FLIGHT_INGESTION_WORKERS` | `ingestion.workers` |
| `FLIGHT_INGESTION_REDACTED_KEYS` | `ingestion.redacted_keys` as comma-separated exact keys |
| `FLIGHT_DATABASE_URL` | `database.url` |
| `FLIGHT_DATABASE_MAX_CONNECTIONS` | `database.max_connections` |
| `FLIGHT_DATABASE_MIN_CONNECTIONS` | `database.min_connections` |
| `FLIGHT_DATABASE_MAX_CONNECTION_LIFETIME` | `database.max_connection_lifetime` |
| `FLIGHT_DATABASE_MAX_CONNECTION_IDLE_TIME` | `database.max_connection_idle_time` |
| `FLIGHT_DATABASE_HEALTH_CHECK_PERIOD` | `database.health_check_period` |
| `FLIGHT_DATABASE_READINESS_TIMEOUT` | `database.readiness_timeout` |
| `FLIGHT_MIGRATIONS_PATH` | `database.migrations_path` |
| `FLIGHT_SHUTDOWN_TIMEOUT` | `shutdown_timeout` |
| `FLIGHT_LOG_LEVEL` | `log_level` |

`database.url` is required. Connection bounds must satisfy `0 <= min_connections <= max_connections` and reserve one connection per ingestion worker plus two. OTLP listeners must differ from each other and from the management listener. Project/environment IDs must be UUIDs, the queue must hold at least one full batch, and all timeouts and limits must be positive and within safe upper bounds. This prevents a valid-looking configuration from allocating an unbounded channel or worker set before listening. Effective configuration is never logged because it may contain secrets.

Phase 2 uses one statically configured project and environment for a trusted Collector boundary. Flight creates that scope transactionally with its first accepted batch. Do not expose ports `4317` or `4318` publicly; authenticated key-to-project mapping is delivered in Phase 4.

Configuration compatibility follows expand/deprecate/remove: add optional fields within version 1, warn before changing meaning, and introduce version 2 only for incompatible semantics.

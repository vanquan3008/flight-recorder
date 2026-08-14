# Ingestion

OTLP ingestion is implemented in Phase 2. Flight accepts traces and logs over gRPC `:4317` and OTLP/HTTP protobuf or JSON `:4318`.

```text
OTLP decode -> normalize -> validate -> redact -> bounded queue -> batch persist
                                                         |
                                                         v
                                      durable incident work (Phase 3)
```

The OTLP response path never performs expensive correlation. Requests have compressed/decoded byte limits, record limits, and bounded attribute shapes. Sensitive attribute keys are redacted before queue admission. Invalid records use OTLP partial success where supported. Queue admission waits for a finite duration; overload is returned explicitly and counted.

The durability contract acknowledges only after PostgreSQL commit. Batch inserts are idempotent by a deterministic UUIDv7 Event ID derived from source identity and timestamp. A retry can therefore repeat an already committed record without inserting it twice. Phase 3 adds durable incident-work claims over these persisted events.

OTLP spans have stable trace/span identity. OTLP logs have no universal record ID, so Flight derives one from timestamp, service, scope, trace/span context, severity, body, and bounded attributes. Two byte-for-byte equivalent log records with all of those fields equal intentionally collapse as the V1 at-least-once tradeoff.

Phase 2 uses one statically configured project/environment identity and transactionally creates that scope with the first accepted batch. OTLP request data cannot select a project. Ports must remain behind a trusted network boundary until Phase 4 adds authenticated identity mapping.

Metrics ingestion, Kafka, Redis, and ClickHouse are not Phase 2 dependencies. Organizations can configure their Collector to export to Flight and an existing backend in parallel.

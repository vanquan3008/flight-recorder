# Storage

PostgreSQL is the system of record. Migration `000001_core` creates project ownership, environments, services, events, incidents, occurrences, timeline links, artifact metadata, API-key hashes, replay audit records, and query-driven indexes. Phase 2 actively writes projects, environments, services, and canonical events; later phases adopt the remaining tables.

Typed columns hold ownership, status, severity, correlation IDs, timestamps, and filter fields. JSONB is limited to dynamic Event attributes and replay comparison output. Every cross-aggregate relationship carries project identity through composite foreign keys.

The current schema is explicitly migrated and readiness requires the latest clean migration version. Schema rollout follows expand/application/contract compatibility; destructive migrations never run automatically at server startup.

Large bodies will use bounded blob storage in Phase 5. PostgreSQL stores only opaque keys, size, type, SHA-256, and truncation metadata. Filesystem storage is the first implementation; object storage waits for a multi-replica or durability requirement.

ClickHouse is considered only after measured retention volume or query latency exceeds the documented PostgreSQL capacity envelope.

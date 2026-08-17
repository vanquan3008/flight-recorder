# Incident model

> **Status:** Phase 3 — designed, **not yet implemented**. No `internal/incident/` package exists in the current codebase.

Incident is Flight's central aggregate. It summarizes equivalent failures and references immutable event evidence rather than duplicating telemetry.

V1 candidates are deterministic: HTTP server status at least 500, exception Event, or ERROR/FATAL log. Fingerprints normalize service, route/operation, error type, and message whitespace/case. AI and fuzzy clustering are not part of V1.

Deduplication is scoped by project and environment. Phase 3 uses a PostgreSQL transaction-scoped advisory lock derived from project/environment/fingerprint, then creates an incident or atomically adds an occurrence inside the configured window. This is required for correctness across replicas.

Statuses are `OPEN`, `INVESTIGATING`, `RESOLVED`, and `IGNORED`; severities are `INFO`, `WARNING`, `ERROR`, and `CRITICAL`. Both persist as stable strings. State transitions are validated in the domain service, not handlers.

Timeline membership is stored in `incident_events`. Reads join canonical events and order by timestamp plus ID, calculating offsets from the earliest related event.

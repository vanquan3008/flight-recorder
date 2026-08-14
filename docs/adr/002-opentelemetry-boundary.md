# ADR-002: OpenTelemetry integration boundary

## Context

Flight must integrate without proprietary application logging APIs or replacement of existing observability stacks.

## Decision

Accept OTLP traces and logs through standard gRPC/HTTP endpoints and recommend Collector fan-out.

## Consequences

Existing instrumentation remains useful. OTLP versioning and hostile-input limits belong entirely to ingestion adapters.

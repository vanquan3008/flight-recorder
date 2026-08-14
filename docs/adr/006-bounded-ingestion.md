# ADR-006: Bounded asynchronous ingestion

## Context

Correlation must not block OTLP requests, while unbounded goroutines or queues can exhaust memory.

## Decision

Use a bounded admission queue, fixed batch workers, finite waits, explicit overload responses, and durable post-persistence incident work.

## Consequences

Capacity and loss behavior are visible and configurable. Overload can reject telemetry but cannot silently grow memory without bound.

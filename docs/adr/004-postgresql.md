# ADR-004: PostgreSQL for V1

## Context

V1 needs transactional deduplication, metadata queries, events, and familiar operations; load limits are not yet measured.

## Decision

Use PostgreSQL for relational metadata and initial normalized event storage.

## Consequences

One database provides strong transactions and simple local setup. Retention and throughput are measured before partitioning or adopting ClickHouse.

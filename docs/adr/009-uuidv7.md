# ADR-009: UUIDv7 identifiers

## Context

IDs must be generated across replicas, sort roughly by time, and be safe to expose without revealing row counts.

## Decision

Generate UUIDv7 in the application for persisted entities. Ingestion event IDs use the event timestamp and hashed source identity in the UUIDv7 entropy fields so an at-least-once retry keeps the same ID. Add presentation prefixes such as `INC-` only at API/CLI boundaries.

## Consequences

Indexes have better time locality than random UUIDv4 and no central sequence is required. ID generation needs one well-tested implementation dependency or standard-library support when available.

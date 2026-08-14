# ADR-003: Canonical internal Event

## Context

OTLP, optional HTTP capture, and future sources must share correlation and incident logic.

## Decision

Normalize every accepted record to one storage-neutral Event. Domain packages do not depend on protobuf types.

## Consequences

Mapping is explicit and testable. Some source-specific fields live as bounded attributes. Future stores and inputs do not rewrite incident logic.

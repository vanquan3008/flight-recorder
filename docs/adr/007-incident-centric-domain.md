# ADR-007: Incident-centric domain

## Status

Accepted (design). Implementation is pending and tracked as Phase 3; the incident engine is **not yet implemented**.

## Context

Operators need a coherent production failure rather than another raw-signal browser.

## Decision

Make Incident the central aggregate; events remain immutable evidence linked as occurrences and timeline entries.

## Consequences

APIs and CLI optimize for investigation. Deterministic detection/correlation precede replay and no AI clustering is needed in V1.

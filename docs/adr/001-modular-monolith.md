# ADR-001: Modular monolith

## Context

V1 needs ingestion, incidents, API, and replay but has no measured need for independently scaled services.

## Decision

Ship one Go binary with capability-oriented internal packages and PostgreSQL.

## Consequences

Deployment and transactions stay simple. Workload budgets remain separate in-process. Service extraction requires measured scaling or isolation evidence and a new ADR.

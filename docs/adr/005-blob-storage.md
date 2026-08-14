# ADR-005: Blob storage for large payloads

## Context

Captured bodies can be large and sensitive; unlimited byte storage in PostgreSQL would harm core queries and backups.

## Decision

Store only bounded artifact metadata/references in PostgreSQL and implement filesystem blobs first.

## Consequences

Blob lifecycle and database references require reconciliation and joint backup. S3/MinIO wait for a real deployment need.

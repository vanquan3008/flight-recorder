# ADR-008: Deny-by-default replay

## Context

Captured traffic can contain credentials and arbitrary target URLs create SSRF and production-impact risk.

## Decision

Disable replay until redacted capture exists; then require allowlisted targets, IP/DNS/redirect checks, sensitive-header stripping, bounds, and audit.

## Consequences

Some convenient targets require explicit policy. Application checks must be reinforced with deployment egress controls.

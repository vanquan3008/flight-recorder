# TracePilot product roadmap and development checklist

TracePilot helps an operator follow a request across services, recover the evidence behind its result, and investigate failures. For an AI request, the operator should be able to start with the trace ID from a customer report and identify the retrieved data, prompt version, model, tool results, and final response available under the project's capture policy. This includes incorrect answers returned with HTTP `200`.

## Current product

Checked items identify capabilities already present in the repository. Release verification is tracked separately below.

- [x] Receive OpenTelemetry traces, logs, and metrics over OTLP/HTTP and OTLP/gRPC. Scope ingestion with project tokens and environments.
- [x] Browse and filter traces and logs, follow the span waterfall and service flow, inspect attributes and events, and correlate logs by trace or span ID.
- [x] Show service dependencies, request performance, telemetry quality, and incident evidence. Stream scoped logs and incidents through Live Tail.
- [x] Manage users, project memberships and roles, environments, ingest tokens, project retention, and audit events.
- [x] Investigate incidents, deliver signed webhooks when enabled, and replay eligible HTTP requests when enabled.
- [x] Store telemetry in ClickHouse and application records in PostgreSQL. Provide health endpoints, a disposable test stack, and the `loadgen` synthetic load tool.

See the [current application flow](FLOW.md), [application structure](STRUCTURE.md), and [setup and release checks](../README.md#mvp-scope-and-release-criteria).

The refund policy pilot runs `POST /ask` against disposable PostgreSQL and returns the answer with a trace ID. It defaults to the deterministic stub and can call OpenAI Responses in sandbox mode. The OpenTelemetry SDK sends spans, and a Go standard-library sender exports correlated logs to TracePilot over OTLP/HTTP. [Current pilot diagram](FLOW.md#refund-policy-demo) · [Pilot setup](../README.md#try-the-refund-policy-ai-tracing-demo)

## Current direction for request traces and logs

Apply the demo's direct OTLP trace and log flow to one real AI endpoint. The pilot records operational metadata. Later work can add approved model inputs and outputs to the trace inspector. [AI traces and logs implementation plan](TRACEPILOT_TRACES_AND_LOGS_PLAN.md#3-apply-the-flow-to-an-existing-ai-service)

## Source architecture

Keep the Go backend as one modular application and the React console organized by feature. PostgreSQL owns application state and ClickHouse owns telemetry. Reuse existing persistence, authorization, and read paths when a feature fits their contracts. Create an additional process when a requirement or a repeatable measurement justifies it. [Application structure](STRUCTURE.md) · [Application flow](FLOW.md)

| Source area | Responsibility |
| --- | --- |
| `backend/cmd/tracepilot` | Configuration, dependency wiring, and server lifecycle |
| `backend/cmd/aidemo` and `backend/cmd/loadgen` | The reproducible AI producer and the synthetic load tool |
| `backend/internal/application` and `backend/internal/domain` | Use cases and small interfaces in application, pure models and rules in domain |
| `backend/internal/ingest` | OTLP transport, validation, redaction, normalization, and bounded queues |
| `backend/internal/transport/httpapi` | Scoped authorization, REST endpoints, and response contracts |
| `backend/internal/storage` | PostgreSQL and ClickHouse queries, migrations, retention, and persistence |
| `frontend/src/features` and `frontend/src/shared` | Feature workflows and shared API, session, and UI utilities |

Use `backend/.env` and `frontend/.env` as documented for development and production. Update their example files when configuration changes. Use the existing Go, Vite, and container entry points for builds and deployments. [Setup](../README.md#quick-start) · [Production deployment](../README.md#production-deployment)

## P0 Establish the release baseline

- [ ] Run the disposable-stack harness and record the Go, frontend, database integration, and browser outcomes. Keep a reproducible project setup, token creation, OTLP ingestion, and investigation flow.
- [ ] Verify that users and ingest tokens reach only their authorized projects and environments. Include a regular member, a second project, token revocation, and restart with existing data.
- [ ] Rehearse fresh installation, failed migration recovery, and restoring both databases. Run the current ingest and read load workload on representative sample data and record throughput, latency, failures, and shutdown drain time. [Platform release criteria](../README.md#mvp-scope-and-release-criteria)

**Exit criterion.** Existing features have a repeatable baseline against disposable stores, with skipped checks documented.

## P0 Ship direct AI traces and logs

- [x] Add the standard-library sender within `cmd/aidemo`. Use the existing trace context and `/v1/logs` receiver. Verify OTLP JSON encoding, bounded queues, partial acceptance, retry behavior, and graceful shutdown.
- [ ] Emit a small set of sanitized request, retrieval, and model logs. Verify that new records correlate to the correct span and that warning-level failure context preserves current incident counts.
- [ ] Run the policy version `1` and `2` cases. Verify that each response returns a distinct trace ID and that traces and logs can be investigated in the existing console. Add a short support procedure and exercise a database failure, model timeout, and export outage. [Implementation and acceptance checklist](TRACEPILOT_TRACES_AND_LOGS_PLAN.md#1-export-correlated-logs-from-the-demo)

**Exit criterion.** The operator starts with a `200` response that reports 14 days and finds the log showing that version `1` was selected.

## P1 Expand AI and distributed investigation

- [ ] Preserve the two typed usage counters while retaining configured and built-in redaction for credentials. Verify the model selected by the application and the model reported by the provider remain distinguishable.
- [ ] Define which roles and environments can read approved prompt and output data, then test opt-in capture on synthetic fixtures. Keep ordinary request logs metadata-only. Validate retention and masking before using customer content. [AI evidence plan](TRACEPILOT_TRACES_AND_LOGS_PLAN.md#4-add-ai-evidence-in-tracepilot-when-needed)
- [ ] Instrument one outbound internal HTTP call and its receiving service with W3C trace context. Prove that both services' logs and spans appear under one trace. Use the same approach when rolling out to one real AI workflow.
- [ ] Add spans for the steps that run in that workflow, including retrieval, tools, retries, and model fallback. Cover parallel execution, timeouts, cancellations, and streaming behavior when those paths exist. Use the existing inspector until a separate AI view solves a demonstrated operator need.
- [ ] Measure how long large traces take to show the waterfall. Correlated logs can render during span loading. If the span delay affects investigations, render the first page while the remaining pages load and label incomplete service and error counts.

**Exit criterion.** One multi-service trace shows the path of the request and the evidence for its final response. The synthetic capture test proves the access and data handling rules.

## P2 Improve platform operations and onboarding

- [ ] Add runnable onboarding examples and an operator procedure for exporter failures, token rotation, retained-data lookups, and project recovery.
- [ ] Complete account recovery with expiring, single-use credentials. Verify reuse, expiry, and authorization paths.
- [ ] Add scoped incident rules, evaluation previews, and notification routing based on existing incident and outbox use cases. Verify rule overlap, retry effects, and project isolation with disposable stores.
- [ ] Attach answer-quality feedback to the originating trace when the workflow is ready for a pilot. Start with a small set of known bad results and deterministic checks. Compare prompt and model changes against that set before expanding to automated evaluation.

**Exit criterion.** Support can investigate or escalate through the current workflow and distinguish an incorrect answer from an infrastructure failure.

## P3 Measure capacity and harden rollout

- [ ] Use `loadgen` for ingestion and read baselines and the instrumented sample for AI tracing. Record request overhead, log drops, queue depth, export errors, and the delay until both signals are queryable. Increase capacity where repeated runs establish a bottleneck.
- [ ] Rehearse restarts and destination outages. Verify that log export, trace export, and request handling recover under the configured budgets. Establish a sampling and retention policy that matches the support investigation window.
- [ ] Validate backup and restore, deployment readiness, and rollback procedures in disposable environments. Roll out to one project first and collect evidence before wider adoption. [Test harness](../README.md#container-test-harness) · [Measured capacity](../README.md#measured-capacity)

**Exit criterion.** The operator has measured limits, verified recovery procedures, and enough retained evidence to investigate representative support cases.

## Checks for each change

| Affected area | Required evidence |
| --- | --- |
| Go logic | A focused regression test, `go -C backend test ./...`, `go -C backend vet ./...`, and `go -C backend build ./...` |
| Concurrent queue or worker | Tests for saturation, cancellation, and shutdown under the Go race detector |
| PostgreSQL or ClickHouse behavior | Integration tests against disposable stores. Exercise migrations and recovery when the schema changes, and verify existing records and query callers. |
| API and ingestion | Verify the existing route through authentication, validation, persistence, and reads. Update Postman for route or ingestion changes. |
| React | `npm --prefix frontend run check`, `npm --prefix frontend run test`, and `npm --prefix frontend run build`, plus a browser check of the changed workflow |
| Operating flow | Update `README.md`, `FLOW.md`, and the matching Mermaid diagram in `diagrams.html` when the flow changes. Record skipped checks and gaps. |

[Repository working rules](../AGENTS.md) · [Development checks](../README.md#development)

## Earlier pilot reference

The optional dual-export demo verified paired span IDs, two content-capture modes, and queue overflow and recovery with a local Langfuse-compatible endpoint. Its configuration and existing behavior are documented in the demo flow. [Earlier Collector flow](FLOW.md#ai-demo-collector)

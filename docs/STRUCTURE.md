# TracePilot application structure

TracePilot collects OpenTelemetry traces, logs, and metrics to help you monitor service performance and investigate incidents.

| Component | Role |
| --- | --- |
| Go backend (`backend/`) | Receives telemetry, serves the console API, and runs background tasks such as incident detection. |
| React console (`frontend/`) | Lets you explore traces, search logs, monitor performance, and manage incidents by project. |
| PostgreSQL | Stores accounts, projects, tokens, incidents, webhooks, and replays. |
| ClickHouse | Stores logs, traces, and metrics and serves their queries and anomaly signals. |
| Deployment (`deploy/`) | Configures the backend and console for production. |
| AI tracing demo (`backend/cmd/aidemo`) | Reads fixture policies from disposable PostgreSQL, runs a deterministic stub or an opt-in OpenAI sandbox model, and exports OpenTelemetry traces. |
| Demo Collector (`deploy/aidemo`) | Routes operational spans to TracePilot and model observations to Langfuse for the local pilot. |

See the [quick start](../README.md#quick-start), [application flow](FLOW.md), or [AI demo flow](FLOW.md#refund-policy-demo).

# TracePilot application structure

TracePilot collects OpenTelemetry traces, logs, and metrics to help you monitor service performance and investigate incidents.

| Component | Role |
| --- | --- |
| Go backend (`backend/`) | Receives telemetry, serves the console API, and runs background tasks such as incident detection. |
| React console (`frontend/`) | Lets you explore traces, search logs, monitor performance, and manage incidents by project. |
| PostgreSQL | Stores accounts, projects, tokens, incidents, webhooks, and replays. |
| ClickHouse | Stores logs, traces, and metrics and serves their queries and anomaly signals. |
| Deployment (`deploy/`) | Configures the backend and console for production. |

See the [quick start](../README.md#quick-start) or [application flow](FLOW.md).

# Backend layout

```text
cmd/
  tracepilot/          Main TracePilot binary and subcommands.
  tracepilot-agent/    Log-file shipping binary.
internal/
  management/      Authenticated management HTTP API.
  ingest/otlp/     OTLP gRPC and HTTP receivers.
  replay/          Allowed request execution and its request sanitizer.
  postgres/        PostgreSQL persistence and queries.
  pipeline/        Bounded ingest queue and batch workers.
  telemetry/       Internal normalized event model.
  agent/           Tail, checkpoint and disk spool logic.
  config/          Config defaults, loading and validation.
  migrations/      Migration runner.
  metrics/         Prometheus registry.
migrations/         Ordered PostgreSQL schema changes.
```

`cmd` wires dependencies only. Runtime behavior belongs in `internal`; packages outside this backend module cannot import them.

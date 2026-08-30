# Contributing to TracePilot

TracePilot is early-stage. Discuss large changes in an issue before writing code; small fixes may go directly to a pull request.

## Development checks

```bash
make ci
```

PostgreSQL changes must also pass `tracepilot migrate up`, `tracepilot migrate down 1`, and a second `tracepilot migrate up` against a clean PostgreSQL instance.

Keep changes scoped and add the smallest test that protects new behavior. Do not
add a new infrastructure dependency without a measured need.

Never commit credentials or real captured production telemetry. Use synthetic fixtures only.

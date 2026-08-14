# Contributing to Flight

Flight is early-stage. Discuss large changes in an issue before writing code; small fixes may go directly to a pull request.

## Development checks

```bash
gofmt -w .
go vet ./...
go test -race ./...
go build ./...
staticcheck ./...
govulncheck ./...
```

PostgreSQL changes must also pass `flight migrate up`, `flight migrate down 1`, and a second `flight migrate up` against a clean PostgreSQL instance.

Keep changes scoped, add the smallest test that protects new behavior, and update `PLAN.md` when a decision changes architecture or delivery order. Do not add a new infrastructure dependency without an ADR and measured need.

Never commit credentials or real captured production telemetry. Use synthetic fixtures only.

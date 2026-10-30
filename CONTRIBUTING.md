# Contributing to TracePilot

TracePilot is early-stage. Discuss large changes in an issue before writing code; small fixes may go directly to a pull request.

## Development checks

```bash
gofmt -w $(find backend -type f -name '*.go')
go -C backend vet ./...
go -C backend test -race ./...
go -C backend build ./...
cd backend && staticcheck ./...
cd backend && govulncheck ./...
make sqlc-check
```

Frontend changes also need the web checks, and the embedded console must be
rebuilt and committed together with its source (CI fails on a stale or
untracked `backend/internal/webui/dist`):

```bash
npm --prefix frontend ci
npm --prefix frontend run check
npm --prefix frontend run test
npm --prefix frontend run build
```

`make ci` runs the full gate. Browser flows (`npm --prefix frontend run test:e2e`)
need a migrated, seeded disposable database and a running server; see
`docs/DEVELOPMENT.md`.

PostgreSQL changes must also pass `tracepilot migrate up`, `tracepilot migrate down 1`, and a second `tracepilot migrate up` against a clean PostgreSQL instance.

Keep changes scoped and add the smallest test that protects new behavior. Do not add a new infrastructure dependency without a measured need.

Never commit credentials or real captured production telemetry. Use synthetic fixtures only.

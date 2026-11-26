# Working in this repository

- Communicate with the repository owner in Vietnamese. Write Markdown files in English.
- Before starting a new area of development, present two or three practical approaches,
  explain the trade-offs, and recommend one. Continue with work that has already been
  agreed on without asking again.
- The agreed architecture is a Go and React application organized into modules by
  responsibility, with a separate PostgreSQL database. Split processes only when a
  requirement or measurement justifies it.
- Preserve the user's uncommitted changes. Leave resets, commits, pushes, deployments,
  and migrations on real databases to the user unless they explicitly authorize them.

## Source boundaries

- `backend/cmd/tracepilot` owns configuration, dependency wiring, and process lifecycle.
- `backend/internal/application` owns use cases and background workflows through small interfaces.
- `backend/internal/domain` owns models and pure rules and stays independent of adapters and the runtime.
- `backend/internal/ingest` handles OTLP, normalization, and bounded queuing.
- `backend/internal/transport/httpapi` owns REST, the auth boundary, and response contracts.
- `backend/internal/storage` owns SQL, transactions, migrations, and partitions.
- `frontend/src/features` organizes the UI by feature. Reusable code belongs in `shared`.
- Use `backend/.env` for all backend configuration, with `backend/.env.example` as its template.
  The console has its own `frontend/.env.example` for `VITE_*` build variables and the nginx
  backend address. PostgreSQL and ClickHouse run as external services in production and
  are configured separately on development machines. Run the backend with `go run` or an
  image built from `backend/Dockerfile`. Run the console with `npm run dev` or an image
  built from `frontend/Dockerfile`. Production uses `deploy/compose.yaml` for migration,
  the backend, and the console. It reads `backend/.env` and `frontend/.env` directly.
  Use the same configuration file paths for development and production.

## Verification

- Run `make fmt` for Go formatting and import organization; CI enforces `make fmt-check`.
- Keep PostgreSQL application SQL in `backend/db/queries` for sqlc. Keep multiline
  ClickHouse queries and aggregate column fragments in each storage package's
  `sql/*.sql`, embedded by `queries.go`. Only interpolate code-controlled SQL
  fragments; bind request values through typed parameters. Short predicates,
  shared read-column lists, and operational schema/retention statements may stay in Go.
- For logic changes, add a small regression test and run the relevant tests, Go vet, and Go build.
- For frontend changes, run type checking, tests, and the build.
- For SQL and migration changes, run integration tests against a disposable PostgreSQL database.
- Report skipped checks. Unit test results alone cannot establish production readiness.
- Keep `README.md` for setup, development, contribution, and security. Update both
  `docs/FLOW.md` and its Mermaid diagrams in `docs/diagrams.html` when an application flow changes.
  Keep the documentation focused and update `docs/tracepilot.postman_collection.json` when API
  routes or ingestion behavior change.

# Development

## Docker workflow

```bash
make dev
curl http://localhost:8080/health/ready
curl http://localhost:8080/metrics
```

Equivalent commands:

```bash
docker compose up --build -d postgres
docker compose run --rm migrate
docker compose up -d flight
```

After migration, run the opt-in PostgreSQL persistence check against this disposable local database:

```bash
FLIGHT_INTEGRATION_TEST=1 \
FLIGHT_DATABASE_URL='postgres://flight:flight-dev-only@localhost:5432/flight?sslmode=disable' \
go test ./internal/postgres -run TestStoreEventsIdempotentIntegration
```

The command intentionally targets the host port published by Compose. If another
PostgreSQL instance already owns port `5432`, stop it or change the Compose host
port and update this URL before running the check.

Emit and inspect a synthetic trace:

```bash
docker compose up -d demo
curl http://localhost:8090/error
docker compose exec postgres psql -U flight -d flight \
  -c "SELECT event_type, service_name, http_status, status FROM events ORDER BY occurred_at DESC LIMIT 5;"
```

Migrations are explicit:

```bash
flight migrate up
flight migrate version
flight migrate down 1
```

`down` always requires a positive number of steps. Production deployment should normally roll forward; rollback SQL must be reviewed before use.

## Native workflow

Start PostgreSQL, set `FLIGHT_DATABASE_URL`, migrate, then serve:

```bash
export FLIGHT_DATABASE_URL='postgres://flight:flight-dev-only@localhost:5432/flight?sslmode=disable'
go run ./cmd/flight migrate up
go run ./cmd/flight serve --config configs/flight.example.yaml
```

Run checks:

```bash
gofmt -w .
go vet ./...
go test -race ./...
staticcheck ./...
govulncheck ./...
go build ./...
```

Use synthetic telemetry only. Never put copied production payloads, headers, credentials, or database URLs in fixtures or logs.

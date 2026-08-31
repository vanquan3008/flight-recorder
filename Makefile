COMPOSE := docker compose -f deploy/compose.dev.yaml
SQLC_IMAGE := sqlc/sqlc:1.31.1

.PHONY: build build-all test test-race bench-postgres vet staticcheck vulncheck fmt-check sqlc-generate sqlc-check ci run dev migrate migrate-version mockdata clean

build:
	mkdir -p bin
	go -C backend build -o ../bin/tracepilot ./cmd/tracepilot

build-all:
	mkdir -p bin
	go -C backend build -o ../bin/tracepilot ./cmd/tracepilot
	go -C backend build -o ../bin/tracepilot-agent ./cmd/tracepilot-agent

ci: sqlc-check fmt-check vet staticcheck vulncheck test-race build-all

sqlc-generate:
	docker run --rm -v "$$(pwd)/backend:/src" -w /src $(SQLC_IMAGE) generate

sqlc-check: sqlc-generate
	git diff --exit-code -- backend/internal/storage/postgres/sqlc

test:
	go -C backend test ./...

test-race:
	go -C backend test -race ./...

bench-postgres:
	TRACEPILOT_INTEGRATION_TEST=1 go -C backend test -run '^$$' -bench '^BenchmarkStoreEventsIntegration$$' -benchtime=5s ./internal/storage/postgres

vet:
	go -C backend vet ./...

staticcheck:
	cd backend && staticcheck ./...

vulncheck:
	cd backend && govulncheck ./...

fmt-check:
	@test -z "$$(find backend -type f -name '*.go' -exec gofmt -l {} +)" || (find backend -type f -name '*.go' -exec gofmt -l {} + && exit 1)

run:
	go -C backend run ./cmd/tracepilot serve

dev:
	$(COMPOSE) up --build -d postgres
	$(COMPOSE) run --build --rm migrate
	$(COMPOSE) up -d tracepilot

migrate:
	$(COMPOSE) run --rm migrate

migrate-version:
	$(COMPOSE) run --rm migrate migrate version

mockdata:
	$(COMPOSE) run --build --rm tracepilot mockdata

clean:
	go -C backend clean
	rm -rf bin

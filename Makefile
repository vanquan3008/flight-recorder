.PHONY: build build-all test test-race bench-postgres vet staticcheck vulncheck fmt-check ci run dev migrate migrate-version smoke clean

build:
	go -C backend build -o bin/tracepilot ./cmd/tracepilot

build-all:
	go -C backend build -o bin/tracepilot ./cmd/tracepilot
	go -C backend build -o bin/tracepilot-agent ./cmd/tracepilot-agent

ci: fmt-check vet staticcheck vulncheck test-race build-all

test:
	go -C backend test ./...

test-race:
	go -C backend test -race ./...

bench-postgres:
	cd backend && TRACEPILOT_INTEGRATION_TEST=1 go test -run '^$$' -bench '^BenchmarkStoreEventsIntegration$$' -benchtime=5s ./internal/postgres

vet:
	go -C backend vet ./...

staticcheck:
	cd backend && staticcheck ./...

vulncheck:
	cd backend && govulncheck ./...

fmt-check:
	@files="$$(find backend -name '*.go' -type f)"; test -z "$$(gofmt -l $$files)" || (gofmt -l $$files && exit 1)

run:
	go -C backend run ./cmd/tracepilot serve

dev:
	docker compose up --build -d postgres
	docker compose run --build --rm migrate
	docker compose up --build -d tracepilot

migrate:
	docker compose run --rm migrate

migrate-version:
	docker compose run --rm migrate migrate version

smoke:
	bash scripts/compose-smoke.sh

clean:
	go -C backend clean
	rm -rf backend/bin

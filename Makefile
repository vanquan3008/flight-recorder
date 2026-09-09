COMPOSE := docker compose -f deploy/compose.dev.yaml
SQLC_IMAGE := sqlc/sqlc:1.31.1

.PHONY: build build-all test test-race bench-pipeline bench-postgres vet staticcheck vulncheck fmt-check size-check sqlc-generate sqlc-check web-install web-check web-test web-build web-dist-check web-ci ci run dev demo migrate migrate-version migration-roundtrip mockdata clean

WEB_NPM := npm --prefix web

build: web-build
	mkdir -p bin
	go -C backend build -o ../bin/tracepilot ./cmd/tracepilot

build-all: web-build
	mkdir -p bin
	go -C backend build -o ../bin/tracepilot ./cmd/tracepilot
	go -C backend build -o ../bin/tracepilot-agent ./cmd/tracepilot-agent

ci: web-ci sqlc-check fmt-check size-check vet staticcheck vulncheck test-race build-all

web-install:
	$(WEB_NPM) ci

web-check:
	$(WEB_NPM) run check

web-test:
	$(WEB_NPM) run test

web-build:
	$(WEB_NPM) run build

web-dist-check: web-build
	git diff --exit-code -- backend/internal/webui/dist

web-ci: web-install web-check web-test web-dist-check

sqlc-generate:
	docker run --rm -v "$$(pwd)/backend:/src" -w /src $(SQLC_IMAGE) generate

sqlc-check: sqlc-generate
	git diff --exit-code -- backend/internal/storage/postgres/sqlc

test:
	go -C backend test ./...

test-race:
	go -C backend test -race ./...

bench-pipeline:
	go -C backend test -run '^$$' -bench '^BenchmarkSubmitBatch$$' -benchmem ./internal/ingest/pipeline

bench-postgres:
	TRACEPILOT_INTEGRATION_TEST=1 go -C backend test -run '^$$' -bench '^BenchmarkStoreEventsIntegration$$' -benchtime=5s ./internal/storage/postgres

vet:
	go -C backend vet ./...

staticcheck:
	cd backend && go run honnef.co/go/tools/cmd/staticcheck@2026.1 ./...

vulncheck:
	cd backend && go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fmt-check:
	@test -z "$$(find backend -type f -name '*.go' -exec gofmt -l {} +)" || (find backend -type f -name '*.go' -exec gofmt -l {} + && exit 1)

size-check:
	@failed=0; \
	for file in $$(find backend/cmd backend/internal -type f -name '*.go' ! -path '*/sqlc/*'); do \
		lines=$$(wc -l < "$$file"); \
		if [ "$$lines" -gt 300 ]; then echo "$$file: $$lines lines (maximum 300)"; failed=1; fi; \
	done; \
	for file in $$(find web/src -type f \( -name '*.ts' -o -name '*.tsx' -o -name '*.css' \)); do \
		lines=$$(wc -l < "$$file"); \
		if [ "$$lines" -gt 300 ]; then echo "$$file: $$lines lines (maximum 300)"; failed=1; fi; \
	done; \
	test "$$failed" -eq 0

run:
	go -C backend run ./cmd/tracepilot serve

dev:
	$(COMPOSE) up --build -d

demo: dev
	$(COMPOSE) run --build --rm tracepilot mockdata

migrate:
	$(COMPOSE) run --rm migrate

migrate-version:
	$(COMPOSE) run --rm migrate migrate version

migration-roundtrip:
	$(COMPOSE) run --rm migrate migrate up
	$(COMPOSE) run --rm migrate migrate down 1
	$(COMPOSE) run --rm migrate migrate up

mockdata:
	$(COMPOSE) run --build --rm tracepilot mockdata

clean:
	go -C backend clean
	rm -rf bin

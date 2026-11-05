SQLC_IMAGE := sqlc/sqlc:1.31.1

.PHONY: build test test-race bench-pipeline bench-postgres vet staticcheck vulncheck fmt-check size-check sqlc-generate sqlc-vet sqlc-check web-install web-check web-test web-build web-dev web-ci ci run dev migrate migrate-version migration-roundtrip seed clean

FRONTEND_NPM := npm --prefix frontend

build: web-build
	mkdir -p bin
	go -C backend build -o ../bin/tracepilot ./cmd/tracepilot

ci: web-ci sqlc-check fmt-check size-check vet staticcheck vulncheck test-race build

web-install:
	$(FRONTEND_NPM) ci

web-check:
	$(FRONTEND_NPM) run check

web-test:
	$(FRONTEND_NPM) run test

web-build:
	$(FRONTEND_NPM) run build

web-dev:
	$(FRONTEND_NPM) run dev

web-ci: web-install web-check web-test web-build

sqlc-generate:
	docker run --rm -v "$$(pwd)/backend:/src" -w /src $(SQLC_IMAGE) generate

sqlc-vet:
	docker run --rm -v "$$(pwd)/backend:/src" -w /src $(SQLC_IMAGE) vet

sqlc-check: sqlc-generate sqlc-vet
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
	for file in $$(find frontend/src -type f \( -name '*.ts' -o -name '*.tsx' -o -name '*.css' \)); do \
		lines=$$(wc -l < "$$file"); \
		if [ "$$lines" -gt 300 ]; then echo "$$file: $$lines lines (maximum 300)"; failed=1; fi; \
	done; \
	test "$$failed" -eq 0

run:
	go -C backend run ./cmd/tracepilot serve

dev: migrate run

migrate:
	go -C backend run ./cmd/tracepilot migrate up

migrate-version:
	go -C backend run ./cmd/tracepilot migrate version

migration-roundtrip:
	go -C backend run ./cmd/tracepilot migrate up
	go -C backend run ./cmd/tracepilot migrate down 1
	go -C backend run ./cmd/tracepilot migrate up

seed:
	go -C backend run ./cmd/tracepilot seed

clean:
	go -C backend clean
	rm -rf bin

.PHONY: build test test-race vet staticcheck vulncheck fmt-check run dev migrate migrate-version clean

build:
	go build -o bin/flight ./cmd/flight

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

staticcheck:
	staticcheck ./...

vulncheck:
	govulncheck ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

run:
	go run ./cmd/flight serve

dev:
	docker compose up --build -d postgres
	docker compose run --rm migrate
	docker compose up -d flight

migrate:
	docker compose run --rm migrate

migrate-version:
	docker compose run --rm migrate migrate version

clean:
	go clean
	rm -rf bin

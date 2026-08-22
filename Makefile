.PHONY: build build-all test test-race vet staticcheck vulncheck fmt-check ci run dev migrate migrate-version clean

build:
	go build -o bin/flight ./cmd/flight

build-all:
	go build -o bin/flight ./cmd/flight
	go build -o bin/flight-agent ./cmd/flight-agent

ci: fmt-check vet staticcheck vulncheck test-race build-all

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
	docker compose run --build --rm migrate
	docker compose up -d flight

migrate:
	docker compose run --rm migrate

migrate-version:
	docker compose run --rm migrate migrate version

clean:
	go clean
	rm -rf bin

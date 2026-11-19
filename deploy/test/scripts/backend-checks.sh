#!/usr/bin/env bash
# Mirrors the lint and test jobs of .github/workflows/ci.yml.
source /harness/common.sh

step "Copy the repository"
copy_tree /src /work
cd /work

step "gofmt (make fmt-check)"
unformatted=$(find backend -type f -name '*.go' -exec gofmt -l {} +)
if [ -n "$unformatted" ]; then
	printf '%s\n' "$unformatted"
	exit 1
fi

step "File size limit (make size-check)"
failed=0
while IFS= read -r file; do
	lines=$(wc -l <"$file")
	if [ "$lines" -gt 300 ]; then
		echo "$file: $lines lines (maximum 300)"
		failed=1
	fi
done < <(find backend/cmd backend/internal -type f -name '*.go' ! -path '*/sqlc/*'; find frontend/src -type f \( -name '*.ts' -o -name '*.tsx' -o -name '*.css' \))
[ "$failed" -eq 0 ]

cd backend
step "go vet"
go vet ./...

if [ "${STATICCHECK:-1}" = "1" ]; then
	step "staticcheck 2026.1"
	go run honnef.co/go/tools/cmd/staticcheck@2026.1 ./...
fi

step "go build"
go build ./...

step "Migrate the test databases twice (as CI does)"
go build -o /tmp/tracepilot ./cmd/tracepilot
(
	cd /tmp
	./tracepilot migrate status
	./tracepilot migrate clickhouse status
	./tracepilot migrate all
	./tracepilot migrate all
)

step "go test -race with PostgreSQL and ClickHouse integration tests"
# shellcheck disable=SC2086 # GO_TEST_FLAGS is a list of flags
go test -race ${GO_TEST_FLAGS:-} ./...

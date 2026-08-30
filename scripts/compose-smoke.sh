#!/usr/bin/env bash
set -euo pipefail

smoke_timeout_seconds="${TRACEPILOT_SMOKE_TIMEOUT_SECONDS:-600}"
[[ "$smoke_timeout_seconds" =~ ^[1-9][0-9]*$ ]] || {
  echo "TRACEPILOT_SMOKE_TIMEOUT_SECONDS must be a positive integer" >&2
  exit 1
}
if [[ "${TRACEPILOT_SMOKE_TIMEOUT_WRAPPED:-0}" != "1" ]]; then
  command -v timeout >/dev/null 2>&1 || {
    echo "compose smoke requires GNU timeout (coreutils); set up Bash/Linux tooling first" >&2
    exit 1
  }
  export TRACEPILOT_SMOKE_TIMEOUT_WRAPPED=1
  exec timeout --preserve-status --signal=TERM "$smoke_timeout_seconds" bash "$0" "$@"
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
project="tracepilot-smoke-${CI_JOB_ID:-${GITHUB_RUN_ID:-$$}}"
workdir="$(mktemp -d)"
api_key="tracepilot-compose-smoke-key-0000000000001"
smoke_volume="${project}_postgres"

case "$smoke_volume" in
  tracepilot-smoke-*_postgres) ;;
  *) echo "refusing to use unsafe smoke volume name: $smoke_volume" >&2; exit 1 ;;
esac

cleanup() {
  docker compose -p "$project" down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$workdir"
}
trap cleanup EXIT

cd "$root"
export TRACEPILOT_POSTGRES_VOLUME_NAME="$smoke_volume"
export TRACEPILOT_AUTH_BOOTSTRAP_API_KEY="$api_key"
export TRACEPILOT_HTTP_ALLOWED_ORIGINS="http://operator.example.test"
docker compose -p "$project" up --build -d postgres
docker compose -p "$project" run --build --rm migrate
docker compose -p "$project" up --build -d tracepilot

for _ in {1..30}; do
  if curl --fail --silent --show-error http://127.0.0.1:8080/health/ready >/dev/null; then
    break
  fi
  sleep 1
done
curl --fail --silent --show-error http://127.0.0.1:8080/health/ready >/dev/null

printf 'compose-smoke-log\n' > "$workdir/source.log"
go -C backend build -o "$workdir/tracepilot-agent" ./cmd/tracepilot-agent
timeout --preserve-status --signal=TERM 3s "$workdir/tracepilot-agent" \
  --source "$workdir/source.log" \
  --checkpoint "$workdir/checkpoint.json" \
  --endpoint 127.0.0.1:4317 \
  --api-key "$api_key" \
  --service compose-smoke

curl --fail --silent --show-error -H "X-API-Key: $api_key" http://127.0.0.1:8080/api/v1/logs?limit=10 | grep -q 'compose-smoke-log'

timestamp_nanos="$(($(date +%s) * 1000000000))"
curl --fail --silent --show-error \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $api_key" \
  --data "{\"resourceLogs\":[{\"resource\":{\"attributes\":[{\"key\":\"service.name\",\"value\":{\"stringValue\":\"compose-smoke\"}}]},\"scopeLogs\":[{\"logRecords\":[{\"timeUnixNano\":\"$timestamp_nanos\",\"severityNumber\":17,\"severityText\":\"ERROR\",\"body\":{\"stringValue\":\"compose-smoke-incident\"}}]}]}]}" \
  http://127.0.0.1:4318/v1/logs

incident_id=""
for _ in {1..30}; do
  incident_id="$(curl --fail --silent --show-error \
    -H "X-API-Key: $api_key" \
    'http://127.0.0.1:8080/api/v1/incidents?limit=1' | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
  if [[ -n "$incident_id" ]]; then
    break
  fi
  sleep 1
done
test -n "$incident_id"

curl --fail --silent --show-error \
  -X OPTIONS \
  -D "$workdir/preflight.headers" \
  -o /dev/null \
  -H "Origin: http://operator.example.test" \
  -H "Access-Control-Request-Method: PATCH" \
  "http://127.0.0.1:8080/api/v1/incidents/$incident_id"
grep -qi '^Access-Control-Allow-Methods:.*PATCH' "$workdir/preflight.headers"

curl --fail --silent --show-error \
  -X PATCH \
  -H "Origin: http://operator.example.test" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $api_key" \
  --data '{"status":"INVESTIGATING"}' \
  "http://127.0.0.1:8080/api/v1/incidents/$incident_id" | grep -q '"status":"INVESTIGATING"'

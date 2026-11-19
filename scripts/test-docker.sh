#!/usr/bin/env bash
# Runs TracePilot's checks against disposable PostgreSQL and ClickHouse in
# containers: backend lint and tests, frontend checks, the full stack, the
# Playwright suite and, optionally, the capacity load test. Works with Docker
# Compose or `podman compose`.
#
# Environment:
#   STAGES       comma list of backend,frontend,stack,e2e,load (default backend,frontend,stack,e2e)
#   LOAD=1       also run the load stage (same as adding "load" to STAGES)
#   KEEP_STACK=1 leave the containers running for debugging instead of removing them
#   SKIP_BUILD=1 reuse the backend and console images from an earlier run
#   CONTAINER_CLI=docker|podman  force a container CLI
#   TP_TEST_*    see deploy/test/compose.yaml for ports, images and load settings
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
compose_file="$root/deploy/test/compose.yaml"
results="$root/bin/test-docker"
project="${TP_TEST_PROJECT:-tracepilot-test}"
stages=",${STAGES:-backend,frontend,stack,e2e},"
if [ "${LOAD:-0}" = "1" ]; then
	stages="${stages}load,"
fi
export TP_TEST_BACKEND_IMAGE="${TP_TEST_BACKEND_IMAGE:-localhost/tracepilot-test/backend:latest}"
export TP_TEST_CONSOLE_IMAGE="${TP_TEST_CONSOLE_IMAGE:-localhost/tracepilot-test/console:latest}"

if [ -n "${CONTAINER_CLI:-}" ]; then
	cli=$CONTAINER_CLI
elif command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
	cli=docker
elif command -v podman >/dev/null 2>&1; then
	cli=podman
else
	echo "test-docker: Docker with Compose or Podman is required" >&2
	exit 2
fi

build_flags=()
if [ "$cli" = "podman" ]; then
	export PODMAN_COMPOSE_WARNING_LOGS=false
	# Keep image HEALTHCHECKs, which the OCI format would drop.
	build_flags+=(--format docker)
	# The Dockerfiles use short FROM names; resolve them through docker.io when
	# the host defines no unqualified-search registries.
	if [ -z "${CONTAINERS_REGISTRIES_CONF:-}" ] && ! podman info --format '{{json .Registries}}' 2>/dev/null | grep -q '"search"'; then
		export CONTAINERS_REGISTRIES_CONF="$root/deploy/test/registries.conf"
	fi
else
	# Rootful Docker writes reports as root; hand them back to the caller.
	export TP_TEST_HOST_UID="${TP_TEST_HOST_UID:-$(id -u)}"
	export TP_TEST_HOST_GID="${TP_TEST_HOST_GID:-$(id -g)}"
fi

compose() {
	"$cli" compose -p "$project" -f "$compose_file" "$@"
}
all_profiles=(--profile checks --profile stack --profile e2e --profile load)

build_images() {
	if [ "${SKIP_BUILD:-0}" = "1" ]; then
		"$cli" image inspect "$TP_TEST_BACKEND_IMAGE" "$TP_TEST_CONSOLE_IMAGE" >/dev/null
		return
	fi
	"$cli" build "${build_flags[@]}" --target server -t "$TP_TEST_BACKEND_IMAGE" "$root/backend" &&
		"$cli" build "${build_flags[@]}" --target console -t "$TP_TEST_CONSOLE_IMAGE" "$root/frontend"
}

wants() { case "$stages" in *",$1,"*) return 0 ;; *) return 1 ;; esac; }

summary=()
failed=0
record() { # record NAME STATUS
	if [ "$2" -eq 0 ]; then
		summary+=("PASS  $1")
	else
		summary+=("FAIL  $1 (exit $2)")
		failed=1
	fi
}
skip() { summary+=("SKIP  $1 ($2)"); }

run_stage() { # run_stage NAME COMMAND...; returns the command's status
	local name=$1 status=0
	shift
	printf '\n##### %s #####\n' "$name"
	"$@" || status=$?
	record "$name" "$status"
	return "$status"
}

finish() {
	local status=$?
	if [ "$failed" -ne 0 ] || [ "$status" -ne 0 ]; then
		compose "${all_profiles[@]}" logs --no-color --timestamps >"$results/compose.log" 2>&1 || true
		printf '\n##### Container logs (last 200 lines; full log in %s) #####\n' "$results/compose.log"
		tail -n 200 "$results/compose.log" || true
	fi
	if [ "${KEEP_STACK:-0}" = "1" ]; then
		printf '\nKEEP_STACK=1: console http://127.0.0.1:%s, API http://127.0.0.1:%s\n' \
			"${TP_TEST_CONSOLE_PORT:-18081}" "${TP_TEST_API_PORT:-18080}"
		printf 'Remove with: %s compose -p %s -f %s %s down -v --remove-orphans\n' "$cli" "$project" "$compose_file" "${all_profiles[*]}"
	else
		compose "${all_profiles[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
	fi
	printf '\n##### Summary (%s) #####\n' "$cli"
	printf '%s\n' "${summary[@]}"
	printf 'Reports: %s\n' "$results"
	if [ "$failed" -ne 0 ] || [ "$status" -ne 0 ]; then
		exit 1
	fi
}
trap finish EXIT

mkdir -p "$results"
for volume in tracepilot-test-go-mod tracepilot-test-go-build tracepilot-test-npm; do
	"$cli" volume inspect "$volume" >/dev/null 2>&1 || "$cli" volume create "$volume" >/dev/null
done

databases_ok=0
run_stage "databases" compose up -d --wait postgres clickhouse || databases_ok=1

if wants backend; then
	if [ "$databases_ok" -eq 0 ]; then
		run_stage "backend: gofmt, size, vet, staticcheck, build, migrate, go test -race" \
			compose --profile checks run --rm backend-checks || true
	else
		skip "backend" "databases did not start"
	fi
fi

if wants frontend; then
	run_stage "frontend: npm ci, audit, check, test, build" compose --profile checks run --rm frontend-checks || true
fi

stack_ok=1
if wants stack || wants e2e || wants load; then
	if [ "$databases_ok" -ne 0 ]; then
		skip "stack" "databases did not start"
	elif run_stage "images: backend and console" build_images; then
		if run_stage "stack: migrate, seed, serve, console" compose --profile stack up -d --wait tracepilot console; then
			stack_ok=0
		fi
	fi
fi

if wants e2e; then
	if [ "$stack_ok" -eq 0 ]; then
		run_stage "e2e: Playwright" compose --profile stack --profile e2e run --rm e2e || true
	else
		skip "e2e" "stack did not start"
	fi
fi

if wants load; then
	if [ "$stack_ok" -eq 0 ]; then
		run_stage "load: 1,000 records/s for ${TP_TEST_LOAD_DURATION:-60s}" compose --profile stack --profile load run --rm loadgen || true
	else
		skip "load" "stack did not start"
	fi
fi

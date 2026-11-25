#!/usr/bin/env bash
# Creates the stack's one super administrator before the backend starts. A rerun on the same
# volumes finds it already there, and the browser and load tests sign in as it.
source /harness/common.sh

step "Copy the backend sources"
copy_tree /src/backend /work
cd /work

step "Create the super administrator"
if output=$(printf '%s\n' "$TRACEPILOT_TEST_ADMIN_PASSWORD" |
	go run ./cmd/tracepilot create-admin --email "$TRACEPILOT_TEST_ADMIN_EMAIL" --name "Test Admin" --password-stdin 2>&1); then
	printf '%s\n' "$output"
else
	status=$?
	printf '%s\n' "$output" >&2
	grep -Fq 'a super administrator already exists' <<<"$output" || exit "$status"
fi

#!/usr/bin/env bash
# Drives the stack at the README capacity workload and keeps the report in /results.
source /harness/common.sh

step "Copy the backend sources"
copy_tree /src/backend /work
cd /work

step "loadgen: ${LOAD_SPANS_PER_SECOND} spans/s + ${LOAD_LOGS_PER_SECOND} logs/s + ${LOAD_METRIC_POINTS_PER_SECOND} points/s, ${LOAD_READS_PER_SECOND} reads/s across ${LOAD_PROJECTS} projects for ${LOAD_DURATION}"
status=0
go run ./cmd/loadgen \
	-api-url http://tracepilot:8080 \
	-ingest-url http://tracepilot:4318 \
	-spans-per-second "$LOAD_SPANS_PER_SECOND" -spans-per-request 512 \
	-logs-per-second "$LOAD_LOGS_PER_SECOND" -logs-per-request 512 \
	-metric-points-per-second "$LOAD_METRIC_POINTS_PER_SECOND" \
	-reads-per-second "$LOAD_READS_PER_SECOND" -read-window "$LOAD_READ_WINDOW" \
	-projects "$LOAD_PROJECTS" -prefill "$LOAD_PREFILL" -history "$LOAD_HISTORY" \
	-duration "$LOAD_DURATION" \
	-metrics-token "$TRACEPILOT_HTTP_METRICS_TOKEN" 2>&1 | tee /results/loadgen.txt || status=$?
give_back /results
exit "$status"

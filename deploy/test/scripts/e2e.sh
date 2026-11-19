#!/usr/bin/env bash
# Runs the Playwright suite against the console service; reports go to /results.
source /harness/common.sh

step "Copy the console sources"
copy_tree /src/frontend /work
cd /work

step "npm ci"
npm ci --no-audit --no-fund

step "Playwright against ${TRACEPILOT_E2E_BASE_URL}"
export PLAYWRIGHT_HTML_OPEN=never
export PLAYWRIGHT_HTML_REPORT=/results/playwright-report
rm -rf /results/playwright-report /results/playwright-artifacts
status=0
npx playwright test --reporter=list,html --output=/results/playwright-artifacts --trace=retain-on-failure || status=$?
give_back /results
exit "$status"

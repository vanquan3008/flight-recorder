#!/usr/bin/env bash
# Mirrors the frontend job of .github/workflows/ci.yml.
source /harness/common.sh

step "Copy the console sources"
copy_tree /src/frontend /work
cd /work

step "npm ci"
npm ci --no-fund

if [ "${NPM_AUDIT:-1}" = "1" ]; then
	step "npm audit --audit-level=moderate"
	npm audit --audit-level=moderate
fi

step "Type check"
npm run check

step "Unit tests"
npm run test

step "Production build"
npm run build

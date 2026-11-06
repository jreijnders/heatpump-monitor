#!/bin/sh
# Runs the Go toolchain in Docker (no local Go needed), inside ingester/.
# Usage: scripts/go.sh test ./internal/reading/
#        TEST_DATABASE_URL=postgres://postgres:test@host.docker.internal:55433/heatpump?sslmode=disable scripts/go.sh test ./internal/store/
set -eu
cd "$(dirname "$0")/../ingester"
exec docker run --rm \
    -v "$PWD":/src -w /src \
    -v heatpump-gomod:/go/pkg/mod -v heatpump-gocache:/root/.cache/go-build \
    -e TEST_DATABASE_URL="${TEST_DATABASE_URL:-}" \
    golang:1.26-alpine go "$@"

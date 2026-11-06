#!/bin/sh
# Starts a throwaway TimescaleDB with the repo schema applied, for Go integration tests.
# Usage: scripts/test-db.sh up <name> <port>    (prints TEST_DATABASE_URL for scripts/go.sh)
#        scripts/test-db.sh migrate <name>      (re-apply db/schema/*.sql)
#        scripts/test-db.sh down <name>
# Role passwords in the test DB: ingester=ingester, grafana_ro=grafana_ro, claude_ro=claude_ro.
set -eu
cmd="$1"; name="hpm-test-$2"
repo="$(cd "$(dirname "$0")/.." && pwd)"
image=timescale/timescaledb:2.28.2-pg17

migrate() {
    docker run --rm --network container:"$name" \
        -v "$repo/db/schema":/schema:ro -v "$repo/db/migrate.sh":/migrate.sh:ro \
        -e PGHOST=127.0.0.1 -e PGUSER=postgres -e PGPASSWORD=test -e PGDATABASE=heatpump \
        -e INGESTER_PASSWORD=ingester -e GRAFANA_RO_PASSWORD=grafana_ro -e CLAUDE_RO_PASSWORD=claude_ro \
        "$image" /bin/sh /migrate.sh
}

case "$cmd" in
up)
    port="$3"
    docker rm -f "$name" >/dev/null 2>&1 || true
    docker run -d --name "$name" -p "127.0.0.1:$port:5432" \
        -e POSTGRES_PASSWORD=test -e POSTGRES_DB=heatpump "$image" >/dev/null
    until docker exec "$name" pg_isready -U postgres -d heatpump -h 127.0.0.1 >/dev/null 2>&1; do sleep 1; done
    sleep 2 # initdb restarts the server once after pg_isready first succeeds
    until docker exec "$name" pg_isready -U postgres -d heatpump -h 127.0.0.1 >/dev/null 2>&1; do sleep 1; done
    migrate
    echo "TEST_DATABASE_URL=postgres://postgres:test@host.docker.internal:$port/heatpump?sslmode=disable"
    ;;
migrate) migrate ;;
down) docker rm -f "$name" >/dev/null ;;
*) echo "usage: $0 up|migrate|down <name> [port]" >&2; exit 2 ;;
esac

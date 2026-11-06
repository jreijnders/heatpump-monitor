#!/bin/sh
# Applies db/schema/*.sql in filename order. Every file is idempotent, so this runs on every `up`.
# Connection comes from libpq env vars (PGHOST, PGUSER, PGPASSWORD, PGDATABASE).
set -eu

SCHEMA_DIR="${SCHEMA_DIR:-/schema}"

for f in "$SCHEMA_DIR"/*.sql; do
    echo "db-migrate: applying $(basename "$f")"
    psql -X -q -o /dev/null -v ON_ERROR_STOP=1 \
        -v ingester_password="$INGESTER_PASSWORD" \
        -v grafana_ro_password="$GRAFANA_RO_PASSWORD" \
        -v claude_ro_password="$CLAUDE_RO_PASSWORD" \
        -f "$f"
done

echo "db-migrate: done"

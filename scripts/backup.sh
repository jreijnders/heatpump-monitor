#!/bin/sh
# Nightly pg_dump loop for the compose db-backup service (timescale image).
# Env: PGHOST PGUSER PGPASSWORD PGDATABASE, BACKUP_DIR (default /backups),
#      BACKUP_KEEP_DAYS (default 14), BACKUP_HOUR (default 03, container time = UTC).
# Usage: backup.sh          loop forever, dump daily at BACKUP_HOUR:00
#        backup.sh once     single dump now (also BACKUP_NOW=1); for tests/pre-restore snapshots
# Restore: see README (timescaledb_pre_restore / pg_restore / timescaledb_post_restore).
set -eu

BACKUP_DIR="${BACKUP_DIR:-/backups}"
KEEP_DAYS="${BACKUP_KEEP_DAYS:-14}"
HOUR="${BACKUP_HOUR:-03}"

log() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) backup: $*"; }

dump_once() {
    mkdir -p "$BACKUP_DIR"
    name="heatpump-$(date +%Y%m%d-%H%M%S).dump"
    tmp="$BACKUP_DIR/.$name.tmp"
    log "dumping $PGDATABASE to $name"
    if pg_dump -Fc -f "$tmp"; then
        mv "$tmp" "$BACKUP_DIR/$name"
        log "done ($(wc -c <"$BACKUP_DIR/$name") bytes)"
    else
        rm -f "$tmp"
        log "pg_dump FAILED"
        return 1
    fi
    find "$BACKUP_DIR" -maxdepth 1 -name 'heatpump-*.dump' -mtime "+$KEEP_DAYS" -print -delete |
        while read -r f; do log "deleted old $f"; done
}

seconds_until_next() {
    # epoch seconds mod 86400 = seconds since 00:00 UTC
    today=$(($(date +%s) % 86400))
    delta=$((${HOUR#0} * 3600 - today))
    [ "$delta" -le 0 ] && delta=$((delta + 86400))
    echo "$delta"
}

if [ "${BACKUP_NOW:-}" = 1 ] || [ "${1:-}" = once ]; then
    dump_once
    exit
fi

while :; do
    wait_s=$(seconds_until_next)
    log "next dump in ${wait_s}s (at ${HOUR}:00 UTC)"
    sleep "$wait_s"
    dump_once || log "will retry tomorrow"
done

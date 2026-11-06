#!/bin/sh
# SSH tunnel to the server's TimescaleDB: localhost:15432 -> server 127.0.0.1:5432.
# Usage: scripts/tunnel.sh
# Env: TUNNEL_HOST (default jeroen@192.168.1.40), LOCAL_PORT (15432), REMOTE_PORT (5432).
set -eu
exec ssh -N -L "${LOCAL_PORT:-15432}:127.0.0.1:${REMOTE_PORT:-5432}" "${TUNNEL_HOST:-jeroen@192.168.1.40}"

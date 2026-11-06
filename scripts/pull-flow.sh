#!/bin/sh
# Export the Gateway's Node-RED flows to nodered/flows.json (pretty-printed for diffs).
# Usage: NODERED_URL=http://<pi>:1880 [NODERED_TOKEN=...] scripts/pull-flow.sh
set -eu
: "${NODERED_URL:?set NODERED_URL, e.g. http://192.168.1.x:1880}"
out="$(cd "$(dirname "$0")/.." && pwd)/nodered/flows.json"
mkdir -p "$(dirname "$out")"
tmp="$(mktemp)"
raw="$(mktemp)"
trap 'rm -f "$tmp" "$raw"' EXIT

if [ -n "${NODERED_TOKEN:-}" ]; then
    curl -fsS -H "Authorization: Bearer $NODERED_TOKEN" -H "Node-RED-API-Version: v1" -o "$raw" "$NODERED_URL/flows"
else
    curl -fsS -H "Node-RED-API-Version: v1" -o "$raw" "$NODERED_URL/flows"
fi
if command -v jq >/dev/null 2>&1; then
    jq . "$raw" >"$tmp"
else
    python3 -m json.tool "$raw" >"$tmp"
fi
mv "$tmp" "$out"
echo "wrote $out"

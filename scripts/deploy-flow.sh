#!/bin/sh
# Deploy nodered/flows.json to the Gateway's Node-RED (full deploy).
# Usage: NODERED_URL=http://<pi>:1880 [NODERED_TOKEN=...] scripts/deploy-flow.sh
set -eu
: "${NODERED_URL:?set NODERED_URL, e.g. http://192.168.1.x:1880}"
file="$(cd "$(dirname "$0")/.." && pwd)/nodered/flows.json"
[ -f "$file" ] || { echo "missing $file" >&2; exit 1; }

echo "Rollback: git checkout <baseline> -- nodered/flows.json && scripts/deploy-flow.sh" >&2
if [ -n "${NODERED_TOKEN:-}" ]; then
    curl -fsS -X POST -H "Authorization: Bearer $NODERED_TOKEN" -H "Content-Type: application/json" \
        -H "Node-RED-API-Version: v1" -H "Node-RED-Deployment-Type: full" --data-binary @"$file" "$NODERED_URL/flows"
else
    curl -fsS -X POST -H "Content-Type: application/json" \
        -H "Node-RED-API-Version: v1" -H "Node-RED-Deployment-Type: full" --data-binary @"$file" "$NODERED_URL/flows"
fi
echo
echo "deployed $file"

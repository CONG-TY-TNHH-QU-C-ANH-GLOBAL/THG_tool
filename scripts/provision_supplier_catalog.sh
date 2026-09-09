#!/usr/bin/env bash
# Provision (or refresh) a supplier_catalog knowledge source and run one sync.
#
# The source config is supplied by the operator as a JSON file — this script
# never carries queries of its own, because a hardcoded query list would bake a
# single vertical into the platform.
#
# Usage:
#   API_BASE=https://app.thgfulfill.com \
#   ADMIN_TOKEN=<admin JWT> \
#   scripts/provision_supplier_catalog.sh path/to/supplier_catalog.json
#
# Example config (see docs/engineering/SUPPLIER_SOURCING.md):
#   {
#     "base_url": "https://pricingtool.thgfulfill.com",
#     "secret_file": "/etc/thg-scraper/pricing_hub_key",
#     "max_api_calls": 40,
#     "detail_per_query": 4,
#     "queries": [{ "q": "áo hoodie nỉ bông unisex", "platform": "1688", "size": 20 }]
#   }
#
# Assets land in `pending`. Approve them in the Product Explorer before the
# runtime may retrieve them.
set -euo pipefail

CONFIG_FILE="${1:-}"
: "${API_BASE:?set API_BASE to the workspace API origin}"
: "${ADMIN_TOKEN:?set ADMIN_TOKEN to an admin JWT for the target org}"
LABEL="${LABEL:-Supplier catalog (1688/Taobao)}"

if [[ -z "$CONFIG_FILE" || ! -f "$CONFIG_FILE" ]]; then
  echo "usage: $0 <connection-config.json>" >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required" >&2
  exit 2
fi

if ! jq -e '.max_api_calls > 0' "$CONFIG_FILE" >/dev/null; then
  echo "error: config must set a positive max_api_calls (the per-run upstream ceiling)" >&2
  exit 2
fi

api() {
  local method="$1" path="$2"
  shift 2
  curl -fsS -X "$method" "${API_BASE}${path}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" "$@"
}

echo "==> Creating/updating source: ${LABEL}"
body="$(jq -n --arg label "$LABEL" --slurpfile cfg "$CONFIG_FILE" \
  '{type: "supplier_catalog", label: $label, sync_policy: "manual", connection_config: $cfg[0]}')"

source_id="$(api POST /api/knowledge/sources -d "$body" | jq -r '.id // .ID // empty')"
if [[ -z "$source_id" ]]; then
  echo "error: could not read the created source id from the API response" >&2
  exit 1
fi
echo "    source_id=${source_id}"

# The API marshals the Go SyncResult without json tags, so the keys are the
# exported field names under "result".
echo "==> Syncing (bounded by max_api_calls in the config)"
api POST "/api/knowledge/sources/${source_id}/sync" \
  | jq '.result | {seen: .AssetsSeen, indexed: .AssetsUpdated, rejected: .AssetsRejected, errors: .Errors}'

cat <<'NEXT'

==> Done. Assets are in `pending` and are NOT retrievable yet.
    Open the Product Explorer and approve the ones worth keeping.
    Re-run this script later to refresh prices; it updates in place.
NEXT

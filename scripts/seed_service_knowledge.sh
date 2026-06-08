#!/usr/bin/env bash
# Seed a tenant's SERVICE knowledge (P2b) via POST /api/knowledge/seed-service.
# Production-safe path — no DevTools console, no hardcoded data in the binary.
#
# Usage (run on the server):
#   THG_BASE="https://your-host" THG_TOKEN="<admin jwt>" ./scripts/seed_service_knowledge.sh
#
# Idempotent (re-run updates the same source). Tenant-scoped by the token's org.
set -euo pipefail
: "${THG_BASE:?set THG_BASE}"
: "${THG_TOKEN:?set THG_TOKEN}"

# Service capabilities (domain DATA — edit freely).
read -r -d '' CSV <<'CSV' || true
title,description
US Fulfillment,"Fulfillment từ kho VN/CN về kho US, giao 3-5 ngày, tuân thủ rule TikTok Shop, phí từ $4.5/order"
Sourcing VN/CN,"Tìm và gom nguồn hàng từ Việt Nam/Trung Quốc, tối ưu chi phí đầu vào"
Kho US,"Kho tại US (CA/TX), giao nội địa nhanh, phù hợp seller TikTok Shop US"
TikTok Shop Compliant Shipping,"Hỗ trợ tracking và thời gian giao hàng phù hợp rule TikTok Shop, giảm rủi ro vi phạm giao hàng"
CSV

# jq builds a safe JSON body (handles newlines/quotes in the CSV).
BODY=$(jq -n --arg csv "$CSV" '{label:"THG Service Knowledge", asset_type:"sales_playbook", csv:$csv, approve:true}')
echo "Seeding service knowledge..."
curl -sS -X POST "$THG_BASE/api/knowledge/seed-service" \
  -H "Authorization: Bearer $THG_TOKEN" -H "Content-Type: application/json; charset=utf-8" \
  -d "$BODY" | jq .

# Optional CTA as its own type.
CTA_BODY=$(jq -n '{label:"THG CTA", asset_type:"cta", csv:"title,description\nInbox CTA,\"Inbox để THG Fulfill khảo sát sản phẩm và gửi phương án fulfillment/sourcing phù hợp\"", approve:true}')
curl -sS -X POST "$THG_BASE/api/knowledge/seed-service" \
  -H "Authorization: Bearer $THG_TOKEN" -H "Content-Type: application/json; charset=utf-8" \
  -d "$CTA_BODY" | jq .

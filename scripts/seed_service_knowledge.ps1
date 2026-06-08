# Seed a tenant's SERVICE knowledge (P2b) via the admin endpoint
# POST /api/knowledge/seed-service. This is the production-safe path — NO DevTools
# console, NO hardcoded data in the app binary: the domain knowledge lives HERE.
#
# Usage (PowerShell):
#   $env:THG_BASE  = "https://your-host"          # app base URL
#   $env:THG_TOKEN = "<admin access token>"        # an admin JWT (DevTools Network -> any /api request -> Authorization: Bearer <token>)
#   ./scripts/seed_service_knowledge.ps1            # seeds + approves for the token's org
#
# Idempotent: re-running updates the same "THG Service Knowledge" source instead
# of duplicating. Tenant-scoped by the token's org. Does not touch the catalog.
param(
  [string]$Base    = $env:THG_BASE,
  [string]$Token   = $env:THG_TOKEN,
  [bool]  $Approve = $true
)
if ([string]::IsNullOrWhiteSpace($Base) -or [string]::IsNullOrWhiteSpace($Token)) {
  Write-Error "Set THG_BASE and THG_TOKEN first."; exit 1
}

# Service capabilities (domain DATA — edit freely). Single-quoted here-string =
# literal, so the $ in prices is not interpreted.
$csv = @'
title,description
US Fulfillment,"Fulfillment từ kho VN/CN về kho US, giao 3-5 ngày, tuân thủ rule TikTok Shop, phí từ $4.5/order"
Sourcing VN/CN,"Tìm và gom nguồn hàng từ Việt Nam/Trung Quốc, tối ưu chi phí đầu vào"
Kho US,"Kho tại US (CA/TX), giao nội địa nhanh, phù hợp seller TikTok Shop US"
TikTok Shop Compliant Shipping,"Hỗ trợ tracking và thời gian giao hàng phù hợp rule TikTok Shop, giảm rủi ro vi phạm giao hàng"
'@

$body = @{
  label      = "THG Service Knowledge"
  asset_type = "sales_playbook"
  csv        = $csv
  approve    = $Approve
} | ConvertTo-Json -Depth 5

$resp = Invoke-RestMethod -Method Post -Uri "$Base/api/knowledge/seed-service" `
  -Headers @{ Authorization = "Bearer $Token" } `
  -ContentType "application/json; charset=utf-8" -Body $body
Write-Host "Service knowledge seeded:" -ForegroundColor Green
$resp | ConvertTo-Json -Depth 5

# Optional: seed the CTA as its own asset type so it can ground as the CTA slot.
$ctaCsv = @'
title,description
Inbox CTA,"Inbox để THG Fulfill khảo sát sản phẩm và gửi phương án fulfillment/sourcing phù hợp"
'@
$ctaBody = @{ label = "THG CTA"; asset_type = "cta"; csv = $ctaCsv; approve = $Approve } | ConvertTo-Json -Depth 5
$ctaResp = Invoke-RestMethod -Method Post -Uri "$Base/api/knowledge/seed-service" `
  -Headers @{ Authorization = "Bearer $Token" } `
  -ContentType "application/json; charset=utf-8" -Body $ctaBody
Write-Host "CTA seeded:" -ForegroundColor Green
$ctaResp | ConvertTo-Json -Depth 5

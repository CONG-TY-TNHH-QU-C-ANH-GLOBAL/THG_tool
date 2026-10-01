---
doc_type: engineering
status: active
owner: platform
last_reviewed: 2026-10-01
related_pr_or_issue: supplier-sourced-lead-suggestions
---

# Supplier sourcing (1688 / Taobao) in lead suggestions

Operator lead notices select one matching offer. Explicit personalization
requests favor an in-stock POD catalog item; wholesale/import requests favor a
sourceable marketplace item. Without a clear wholesale signal, a matching POD
item has priority over a supplier item.
If an explicit POD request has no matching catalog item, a marketplace item may
be shown as an alternative product source. The draft must say customization
still needs confirmation; a seller listing is not proof that printing is offered.
For a Dropship lead, the operator-facing suggested reply delivered to the
`THG_Sale_Lead` Telegram group includes the source price as a labelled
reference and a shipping reference only when the selected lane can be priced.
It is a draft for sales staff to review, not an automatically posted quote.

## Retrieval and on-demand fallback

The upstream marketplace data comes from Elim (`openapi.elim.asia`), reached
only through the THG Pricing Hub worker, which owns the API key and the D1
response cache. The Pricing Hub documents the current Elim Free plan as **100
requests for the entire 2026-08-07 to 2036-08-07 plan period**, shared with the
human quoting tool. Check the live balance through `GET /api/scrape/quota`
before a sync; do not treat this as a monthly allowance. One call per crawled
lead would exhaust it quickly and also disrupt sales quoting.

A `supplier_catalog` knowledge source can index curated items once. Per-lead
matching first uses approved KnowledgeOS assets at zero upstream cost. Indexed
marketplace titles must still cover the requested product phrase, including
qualifiers such as the target animal/model. An
approved item is only reusable for a post with an explicit marketplace URL
when it has that exact URL. Otherwise, the post's URL is looked up directly
by detail. Without a usable indexed offer or URL,
the runtime extracts a short product phrase, including the blank product in an
explicit POD request when the company catalog has no match. It searches the
named marketplace when specified (otherwise 1688 then Taobao), requires the
result title to cover the meaningful product words, and fetches up to two
detail records through Pricing Hub when the first match has no usable price or
does not describe the requested item. It
rejects vague posts and unrelated/unsafe links. Pricing Hub owns the shared D1
cache and Elim quota accounting; at most two searches and two details are made
for one lead; a named marketplace uses one search and at most two details. A
live lookup requires `PRICING_HUB_INTEGRATION_KEY` or
`/etc/thg-scraper/pricing_hub_key`; `PRICING_HUB_BASE_URL` defaults to
`https://pricingtool.thgfulfill.com`. With no key, only approved indexed items
are offered. The optional suggestion deadline defaults to 12 seconds.

Explicit English requests such as "dropshipping supplier for this hand massager"
use English titles for search and detail matching. A Chinese marketplace match
for a post requesting a European supplier is labelled as a China-based
alternative, never as a European supplier or an exact image match. When the
post omits quantity or destination, the draft asks for them instead of quoting
an unsupported shipping price.

Matching uses text only, with no image comparison. Every searched or indexed
offer is therefore marked `Similar`: the Vietnamese draft says "mẫu tương tự",
and Telegram adds "mẫu tương tự, sale cần đối chiếu ảnh/mã hàng" to the
supplier line. A 1688/Taobao listing pasted by the lead is treated as that
listing only when the returned marketplace item ID matches the pasted link.
That still does not prove its photo matches the lead's requested model.

When a post asks for a product or a source but neither the catalog nor Pricing
Hub returns a usable match, the suggestion asks only for details missing from
the post (model reference, quantity, destination) plus a Telegram line
"⚠️ Tìm nguồn: chưa tìm được…" for
the sale. If a product description is too vague to search, or the marketplace
API is unavailable, the Telegram note names that state instead of claiming a
completed search found nothing. That draft never contains a link, price or shipping cost. For an
English post that asks for a European supplier, it asks whether a China-based
alternative is acceptable. Posts without a product or sourcing need still get no draft.
If enrichment times out, panics or cannot enter the bounded worker queue, an
enabled org receives a safe ask-for-details draft with a distinct "chưa xử lý
kịp" operator note; this does not claim the marketplace was searched.
These fields go to Telegram only: the CRM snapshot receives the draft in
`suggestedReply`, and its contract is unchanged.

## Moving parts

| Piece | Where |
|---|---|
| Pricing Hub client (search / detail / quota) | `internal/suppliersourcing` |
| Persisted payload schema | `internal/workspace_knowledge/suppliers` |
| Pre-index ingestor | `internal/workspace_knowledge/ingestion/supplier_catalog` |
| Asset type | `assets.AssetSupplierProduct` (`supplier_product`) |
| Source type | `sources.SourceSupplierCatalog` (`supplier_catalog`) |
| Match + facts assembly | `internal/services/facebook/lead_supplier_match.go` |
| Reply generation | Deterministic sourcing copy; AI or fixed fallback for POD |
| Sinks | `telegram/render.Lead`, `crmleadsync` `enrichment.supplier` |
| Shipping reference | CRM `POST /api/integrations/thg-tool/shipping-quote` |

The worker uses the existing `CRM_LEAD_SYNC_KEY` for the shipping reference
request. `CRM_SHIPPING_QUOTE_URL` may override the default CRM endpoint in local
or staging environments. Deploy the CRM endpoint before enabling this worker
version; a missing endpoint only omits the shipping reference.

## Configuration

Both sides need one shared key.

**Pricing Hub** (`THG_pricingtool`):

```bash
echo "<key>" | npx wrangler secret put THG_TOOL_INTEGRATION_KEY
```

It opens `POST /api/scrape/detail`, `POST /api/scrape/search` and
`GET /api/scrape/quota` — nothing else. The caller gets role `service`, so every
admin-gated route stays closed.

**THG AutoFlow**: the key is read by the ingestor from the source's
`connection_config`, via `secret_env` or `secret_file` (exactly one):

```json
{
  "base_url": "https://pricingtool.thgfulfill.com",
  "secret_file": "/etc/thg-scraper/pricing_hub_key",
  "max_api_calls": 40,
  "detail_per_query": 4,
  "queries": [
    { "q": "áo hoodie nỉ bông unisex", "platform": "1688", "size": 20 },
    { "q": "áo thun cotton 270g", "platform": "1688", "size": 20 }
  ],
  "links": [
    "https://detail.1688.com/offer/795570801986.html"
  ],
  "tags": ["apparel", "pod"]
}
```

`max_api_calls` is a **hard ceiling for one run** and is required. A run that
hits it stops cleanly and reports `api_budget_exhausted` in the sync result
rather than continuing. Budget accounting:

- one call per `links` entry (detail);
- one call per query (search), plus up to `detail_per_query` calls to fetch the
  weight and MOQ that search results do not carry.

So the example above costs at most `1 + 2 × (1 + 4) = 11` calls.

Check the remaining plan balance before a run. The per-run cap does not protect
the lifetime balance, and each sync may make upstream calls again after cache
expiry:

```bash
curl -H "x-thg-integration-key: <key>" https://pricingtool.thgfulfill.com/api/scrape/quota
```

## Running a sync

Assets land in `pending` and are **not retrievable until approved** — an
operator reviews them in the Product Explorer, or the CLI approves in bulk:

```bash
DB_PATH=data/scraper.db go run ./cmd/knowledge_sync -org <orgID>
```

(Extend the CLI's source list, or configure the source through
`POST /api/knowledge/sources` with `type: "supplier_catalog"`.)

## Invariants

- A supplier item is a **distinct** offer from the catalog product. It is never
  rendered as a THG product page, and the CRM receives it under its own
  `enrichment.supplier` key. Product title terms must match the post before an
  item is offered. A matching POD item wins; the supplier is the fallback.
- CRM calculates a shipping reference only when the post states quantity and
  destination, the item has a known weight, and a `live`, recently fetched CMS
  international rate card supports its cargo category. The operator sees a
  per-parcel reference, never a bulk total. Ambiguous cargo categories receive
  no automatic number. The old undated Epacket seed is not used by this path.
- The international CN→US card is the applicable public lane for that case.
  Domestic 3PL pricing needs a US warehouse shipment and delivery zone; the
  chính ngạch card is for VN→US cargo. Neither is a substitute for missing
  CN→US parcel facts.
- Destination and cargo terms are matched as whole words. An ordinary name such
  as "anh Nam" is not the United Kingdom, and "kẹo táo" is not apparel. For a
  non-bulk request covering multiple items, one item's weight cannot stand in
  for the packed parcel's weight, so no automatic numeric quote is shown.
- Source numbers are copied; the shipping calculator adds the published
  $0.70 handling fee to the selected CMS weight row. No currency conversion,
  estimated weight, or invented MOQ tier — a missing value is omitted everywhere
  (facts block, Telegram strip, CRM payload).
- The Dropship draft is assembled from verified product and rate fields,
  without another LLM call. The POD prompt uses assembled facts and a short
  fixed fallback. When the destination is known but a numeric rate is not,
  the draft names the CN→destination route and asks to confirm delivery mode
  and parcel details. Every draft is still for staff review before sending.
- Suggestions stay best-effort: any failure in this path leaves lead ingestion
  untouched.
- Taobao ids: since 2026-08 the upstream rejects bare numeric item ids, so pass
  the product URL. `suppliersourcing.Client.Detail` accepts either.

## Current rollout gaps

- The default `knowledge_sync` command only provisions the POD catalog (and
  optionally Training). A `supplier_catalog` source must be configured, synced,
  reviewed, and approved separately before any marketplace item can appear.
- Indexed retrieval only covers approved configured items. Live fallback
  handles explicit purchase phrases when the Pricing Hub key is configured;
  vague posts or results with no verified title/link still produce no offer.
- The old Epacket seed in migration `0019_shipping_rates.sql` remains for
  legacy consumers but is not used by the sourcing endpoint. The CRM cron now
  also imports `cnThuong`, `cnMypham`, `cnPin` and `uspsCnUs` from CMS when they
  are `live`. Runtime behavior still needs staging verification against real
  CMS rows and rate freshness.
- Leader's routing rule: explicit custom logo/design requests go to POD;
  wholesale/import requests go to Dropship even if a generic POD catalog title
  overlaps. Bulk posts can show a clearly labelled per-parcel reference, while
  a total shipment quote still needs
  shipping origin, destination country/zone, cargo
  category, weight/dimensions and fulfillment mode before choosing among
  international, domestic 3PL and chính ngạch tables. Missing inputs require a
  clarification instead of an invented total-shipment quote.

## Production canary when no staging environment exists

This workspace has no confirmed staging deployment. Keep
`LEAD_SUGGESTION_ENABLED=false` until the CRM change has been reviewed, merged
into its current `main`, deployed under its deploy rule, and its CMS rate-card
refresh has been checked. Record the CRM Worker version for rollback before
deploying. Do not deploy the CRM feature branch directly: the shared Worker can
remove other teams' changes.

After CRM is live, verify that its shipping endpoint rejects a missing key and
returns a numeric reference only for a current, `live` rate card and complete
standard-cargo parcel facts. Do not use a production customer post as a test
fixture. Then deploy the THG_tool binaries with their documented backup and
hash checks, leaving suggestions disabled. Configure the existing CRM sync key
and Pricing Hub integration key in their respective secret stores; never put
either key in Git or a Telegram message.

Enable exactly one internal test org in `LEAD_SUGGESTION_ORG_IDS`. Review three
operator notices in `THG_Sale_Lead`: a custom-logo POD request, a wholesale
ordinary-apparel request with quantity and destination, and a wholesale
supplement request. The first must show a matching company catalog item; the
second must show a matching 1688/Taobao link, source price and clearly marked
per-parcel cước only when the CMS card supports it; the third must show the
source offer and route but no invented standard-cargo rate. Check that CRM's
supplier snapshot matches the Telegram offer and that lead delivery works even
when the sourcing or shipping service fails. Stop the canary by setting
`LEAD_SUGGESTION_ENABLED=false`; restore binaries or the CRM Worker version only
if the deployment itself caused a regression. Expand the org allowlist only
after the canary evidence is reviewed.

## Routing contract and remaining inputs

The current runtime passes origin, destination, quantity, shipment mode, cargo
category and unit weight as structured fields. A full shipment quote also needs:
`intent`, `product`, `quantity`, `quantity_unit`, `ship_from`,
`destination_country`, `destination_zone_or_zip`, `fulfillment_mode`,
`cargo_category`, `unit_weight_kg`, and dimensions if the lane bills by volume.
Fields extracted from the post should retain evidence text and confidence;
unmentioned values remain unknown. A source marketplace item establishes China
as the supplier origin, but does not establish where the stock will be held or
where the customer wants delivery.

Route candidates are selected from the three company price surfaces:

| Situation supported by the post | Rate source | Additional facts needed for a number |
|---|---|---|
| Individual parcels from China to the buyer | International CN→destination | Destination, cargo category, weight and any volume rule |
| Stock already in a US warehouse, then domestic delivery | Domestic 3PL | US warehouse/fulfillment plan, destination zone or ZIP, parcel details |
| Formal export from Vietnam | Chính ngạch VN→destination | Vietnam origin, destination, shipment weight/volume and route |
| Bulk import from China | International bulk lane | Total shipment quantity/weight/volume and destination zone |

The public landing page reads pricing from CMS `/api/v1/pricing/{slug}`;
responses include `status`, `version`, and `updated_at`. Only `live` tables
should be quoted. Sales refreshes selected chính ngạch and international grids
from CMS, while domestic 3PL is published from a static landing-page table and
the old bulk CN→US card is a local seed. Bulk bookings on the international
pricing page require a separate quote, so this router explicitly refuses a
numeric total without packing and zone details. Staging verification of the
live CMS row schema and card freshness is still required before rollout.

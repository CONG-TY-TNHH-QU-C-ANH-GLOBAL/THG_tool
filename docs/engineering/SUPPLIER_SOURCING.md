---
doc_type: engineering
status: active
owner: platform
last_reviewed: 2026-09-09
related_pr_or_issue: supplier-sourced-lead-suggestions
---

# Supplier sourcing (1688 / Taobao) in lead suggestions

Operator lead notices can now carry a **second** offer next to the org's own
catalog product: a sourceable marketplace item with its real supplier price,
shipping weight, MOQ and origin.

## Why it is pre-indexed, not looked up per lead

The upstream marketplace data comes from Elim (`openapi.elim.asia`), reached
only through the THG Pricing Hub worker, which owns the API key and the D1
response cache. **The budget is tiny, non-renewing, and shared with the human
quoting tool.**

As measured on 2026-09-11, the Free plan grants **100 requests for the whole
billing period, which runs 2026-08-07 → 2036-08-07** — it does NOT refill
monthly. 71 were left, with a zero credit balance. One call per crawled lead
would exhaust the decade in an afternoon and break quoting for the sales team.

Always read the live number before planning a run; never trust a figure written
in a doc:

```bash
curl -H "x-thg-integration-key: <key>" https://pricingtool.thgfulfill.com/api/scrape/quota
```

So the flow is inverted: a `supplier_catalog` knowledge source indexes a curated
set of items once, and per-lead matching is a local KnowledgeOS retrieval that
costs zero upstream calls. Freshness is traded for predictability — re-run the
sync to refresh prices; the notice shows when a price was captured.

## Moving parts

| Piece | Where |
|---|---|
| Pricing Hub client (search / detail / quota) | `internal/suppliersourcing` |
| Persisted payload schema | `internal/workspace_knowledge/suppliers` |
| Pre-index ingestor | `internal/workspace_knowledge/ingestion/supplier_catalog` |
| Asset type | `assets.AssetSupplierProduct` (`supplier_product`) |
| Source type | `sources.SourceSupplierCatalog` (`supplier_catalog`) |
| Match + facts assembly | `internal/services/facebook/lead_supplier_match.go` |
| Reply generation | `ai.MessageGenerator.GenerateLeadReplySuggestion` |
| Sinks | `telegram/render.Lead`, `crmleadsync` `enrichment.supplier` |

## Configuration

Both sides need one shared key.

**Pricing Hub** (`THG_pricingtool`):

```bash
echo "<key>" | npx wrangler secret put THG_TOOL_INTEGRATION_KEY
```

Done on 2026-09-11: the secret is set on the `thg-pricing-tool` Worker and the
deployed build accepts it (version `dac85391-3049-4145-b237-86438db49f4b`).

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

So the example above costs at most `1 + 2 × (1 + 4) = 11` calls. Against a
remaining balance in the dozens, size `max_api_calls` in single digits for a
pilot and treat a larger index as something to fund by upgrading the Elim plan
first.

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
  `enrichment.supplier` key.
- Numbers are copied, never computed. No currency conversion, no estimated
  weight, no invented MOQ tier — a missing value is omitted everywhere
  (facts block, Telegram strip, CRM payload).
- The reply generator may only quote from the assembled facts block, so a number
  absent there cannot reach a customer.
- Suggestions stay best-effort: any failure in this path leaves lead ingestion
  untouched.
- Taobao ids: since 2026-08 the upstream rejects bare numeric item ids, so pass
  the product URL. `suppliersourcing.Client.Detail` accepts either.

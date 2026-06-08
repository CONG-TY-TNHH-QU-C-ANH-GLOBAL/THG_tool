# Comment Decision — DRY-RUN Evaluation Report

> **STATUS: AWAITING DATA.** This report evaluates the P2a/P2c Knowledge
> Intelligence reasoning on REAL leads BEFORE it is allowed to drive live comment
> text. Fill the `<FILL>` cells from the queries in
> `scripts/comment_decision_dryrun_report.sql`, then apply the decision rule (§9).
> No P2b / P2c-live work until this report is filled and read.

---

## 1. How to produce the data (operator)

The reasoning runs in the queue path ONLY when the env flag is on; it logs +
persists each decision and does **not** change the comment text or execution.

1. Enable the dry-run for the scraper process:
   ```powershell
   $env:THG_COMMENT_REASONING_DRYRUN = "1"
   ```
   (Set it in the environment the `scraper` binary runs under, then start it.)
2. Queue **20–50 comment leads** the normal way (your comment-all flow, e.g.
   `/comment_all_leads` with a limit of ~25, or run it twice). Each lead emits:
   - a log line: `[reasoning-dryrun] org=… intent=… conf=… knowledge_gap=… caps=… products=… proofs=…`
   - a row in `prompt_logs` (`action_taken='comment_decision_dryrun'`,
     `action_args=<decision JSON>`).
3. Run the report queries against the DB (default `data/scraper.db`):
   ```powershell
   sqlite3 data\scraper.db ".read scripts\comment_decision_dryrun_report.sql"
   ```
   (or paste each query into the superadmin SQL endpoint.)

> Turn the flag OFF when done (`Remove-Item Env:\THG_COMMENT_REASONING_DRYRUN`)
> so normal queueing pays no LLM cost.

**Run metadata**

| Field | Value |
|---|---|
| Date run | `<FILL>` |
| Org id(s) | `<FILL>` |
| Build / commit | `<FILL>` |
| OpenAI model (comment) | `<FILL>` (default gpt-4.1) |
| Sources connected on org | `<FILL>` (catalog only? + website/FAQ?) |
| Total decisions (query 0) | `<FILL>` |

---

## 2. Metric 1 — knowledge_gap rate (query 1)

| total | knowledge_gap_true | gap_pct |
|---|---|---|
| `<FILL>` | `<FILL>` | `<FILL>` % |

**Read:** high gap_pct = the agent had no grounded offer to make → the org's
knowledge is too thin (today: catalog-only). This is the primary P2b trigger.

---

## 3. Metric 2 — intent distribution (query 2)

| intent | n | pct |
|---|---|---|
| service_seeking | `<FILL>` | `<FILL>`% |
| product_seeking | `<FILL>` | `<FILL>`% |
| ambiguous | `<FILL>` | `<FILL>`% |
| non_lead | `<FILL>` | `<FILL>`% |

**Read:** mostly `service_seeking` with a catalog-only org explains a high gap
(no service knowledge to ground). A large `ambiguous`/`non_lead` share may mean
weak lead text or a classifier needing calibration.

---

## 4. Metric 3 — average selections + confidence (query 3)

| avg_capabilities | avg_products | avg_proofs | cta_rate | avg_confidence |
|---|---|---|---|---|
| `<FILL>` | `<FILL>` | `<FILL>` | `<FILL>` | `<FILL>` |

**Read:** near-zero caps/proofs with catalog-only is expected. avg_products > 0
on product_seeking leads is the healthy signal that catalog grounding works.

---

## 5. Metric 4 — most-selected assets / SKUs (query 4)

| asset_id | sku | times_selected | example_label |
|---|---|---|---|
| `<FILL>` | `<FILL>` | `<FILL>` | `<FILL>` |
| … | | | |

**Read:** a few assets dominating may indicate retrieval over-fitting to generic
terms; broad spread indicates healthy matching. Cross-check the top assets are
sensible for the leads.

---

## 6. Metric 5 — 10 representative decisions (query 5)

Paste 10 representative `decision_json` values (mix of gap=true/false and across
intents). For each, note whether the selection is sensible.

1. `<FILL decision_json>` — verdict: `<sensible? / wrong offer / gap>`
2. …
10. …

---

## 7. Metric 6 — INVARIANT check (query 6)

high-confidence-but-knowledge_gap rows: **`<FILL>` (MUST be 0)**

**Read:** `recalibrateConfidence` forces confidence=0 when nothing is grounded.
Any row here is a grounding/recalibration **bug** — stop and fix before P2c.

---

## 8. Metric 7 — role / semantic mismatch (query 7)

**7a. Products pitched to a `service_seeking` lead (query 7a):** `<FILL>` rows.

| id | intent | n_products | note |
|---|---|---|---|
| `<FILL>` | service_seeking | `<FILL>` | `<FILL>` |

**Read:** a product SKU offered to a service lead is a semantic mismatch (the
catalog being force-fit). A high count means the reasoning prompt / retrieval
needs tightening (e.g. down-weight products on service intent).

**7b. Capability grounded from a POD_product:** structurally **0 by construction**
— the P2a.1 role guard (`kindAllowedForRole`) drops it before it is recorded.
To measure how often the LLM *attempted* a mis-slot (and was blocked), we must
persist `GroundingStats.OfferDropped` (+ per-role drop reasons) into the dry-run
record. That is a small observation-only addition — **not yet wired**. Decide in
§9 whether to add it before P2b/P2c.

---

## 9. Decision rule (what to do next)

Apply after filling the metrics:

| Finding | Next step |
|---|---|
| **knowledge_gap high** (e.g. > ~50%) | **→ P2b**: connect a real knowledge source (website / FAQ / pricing) via the existing `website`/`notion` ingestor so capability/proof grounding is non-empty. Re-run this report. |
| **Decisions look good** (low gap, sensible selections, invariant clean, low 7a) | **→ P2c**: wire `GenerateCommentV2` so the grounded decision drives the comment text. |
| **Role/semantic mismatch high** (7a high, or odd top assets in §5) | **→ Fix grounding/retrieval FIRST**: tighten the decision prompt and/or intent-weight the searcher (down-weight POD_product on service intent) before P2c. Optionally wire drop-telemetry (§7b) to measure. |
| **Invariant breach** (§7 > 0 rows) | **→ Bug**: fix `recalibrateConfidence`/grounding before anything else. |

> Recommended reading order: §7 (invariant) → §2 (gap) → §8 (mismatch) → §6
> (samples). The invariant must be clean before the others are trusted.

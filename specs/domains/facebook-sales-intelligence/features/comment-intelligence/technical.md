# Comment Intelligence Pipeline — Design Doc (P1–P4)

> **STATUS: DESIGN (rev 2, 2026-06-08).** Design-doc-first per the V2 mandates.
> Author: architecture + code-review pass over the existing `workspace_knowledge`
> retrieval/assembly stack, the `ai` generator, the `outbound`/`coordination`
> ledgers, and the `local-connector-extension` comment executor.
>
> **Direction chosen by founder.**
> - **HYBRID, service-first** (rev 1): comments pitch **Service Offers** first;
>   the 131-row POD catalog is a **Product SKU** catalog only and is NOT forced
>   into every comment.
> - **AUTONOMOUS AGENT DECISION** (rev 2): this is an **Organic Sales Network
>   with autonomous agents**, NOT a CRM draft→approval workflow
>   (`feedback_shared_battlefield_not_crm`). The agent decides and **executes
>   end-to-end**; human review is a **fallback**, not the default path. Execution
>   is governed by a deterministic **Policy Gate**, never a mandatory approval card.
> - **KNOWLEDGE-FIRST, NOT FORM-FIRST** (rev 3, founder directive 2026-06-08):
>   the operator supplies **raw business knowledge** (website, catalog, pricing
>   doc, FAQ, Notion, PDF, case studies, existing assets); the **agent** reasons
>   over it and **synthesizes** the offer / proof / CTA / sales-angle **per lead**.
>   **Capability, Offer, Proof, Case-Study are REASONING OUTPUTS, never
>   operator-authored CRUD types.** Building a Service-Offer / Proof / CTA /
>   Case-Study CRUD whose only purpose is to feed the agent is FORBIDDEN — it
>   turns the system into a CMS. The operator is not a content manager; the agent
>   is the sales executive. Pipeline: **Knowledge Sources → Knowledge Extraction
>   → Reasoning Layer → Comment Decision → Execution.**
>
> #### Two safety invariants this rev adds (non-negotiable)
> - **Grounding / no-fabrication:** every selected capability / product / proof /
>   price in a Comment Decision MUST cite a real ingested `asset_id` (or catalog
>   SKU). The agent may *summarize and select*, never *invent* an offer, price, or
>   proof with no source. This is the knowledge analog of P1b's "real asset" rule
>   and is a Policy-Gate precondition for auto-execute.
> - **Graceful degradation + knowledge-gap nudge:** when the org has thin
>   knowledge (e.g. only the POD catalog connected — which is today's real state,
>   since capability/CTA/proof assets exist only in fixtures), the Reasoning Layer
>   returns empty selections + low confidence + a typed `knowledge_gap` signal
>   that nudges the operator to **connect more sources** (NOT fill a form).

### ⚠️ Hard-Rule reconciliation (architect — read before building)

`CLAUDE.md` carries a binding Hard Rule: **"Default outbound automation to
approval-required."** The autonomous-default vision conflicts with the *literal*
wording. This design reconciles them so neither is violated:

1. **Policy Gate is a safety floor, not an off-switch.** Any decision that fails
   *any* auto-execute precondition (§5) deterministically routes to
   `require_review` or `reject`. Risky/uncertain/unverified actions are *still*
   gated — the rule's safety intent is preserved.
2. **`auto_execute` is an explicit per-org opt-in.** A fresh org ships
   `org_policy.auto_execute_enabled = false` (= approval-required), which keeps
   the Hard Rule literally true at the codebase default. The founder enables it
   on the Organic Sales Network org. → autonomy is a *configuration*, not a
   silent global default.
3. **Recommendation:** founder should amend the `CLAUDE.md` Hard Rule wording to
   *"Outbound automation is governed by the Policy Gate; auto-execute is an
   explicit org opt-in, default off"* so the doc and the rulebook stop
   contradicting. **Until amended, code ships default-off.** This is flagged, not
   decided unilaterally.

---

## 0. Problem statement (grounded in code, not assumption)

A catalog sync of 131 products succeeded, yet auto-comments are effectively
"catalog-unaware" and carry no image, no concrete offer, no price, and no record
of *which Facebook identity* posted them. Investigation established the real
causes — narrower than "there is no matching layer":

1. **A matching layer already exists and is wired in.** `queueLeadOutreach`
   calls `Builder.BuildForLeadWithTrace` per lead
   ([cmd/scraper/outbound_lead_pipeline.go](../../../../../cmd/scraper/outbound_lead_pipeline.go)),
   running a scored `hybrid`+`pgvector` searcher with a full explainability trace
   ([context_builder.go:168](../../../../../internal/workspace_knowledge/runtime/context_builder.go#L168))
   and assembling top hits into a prompt block
   ([context_assembly.go:68](../../../../../internal/workspace_knowledge/assembly/context_assembly.go#L68)).
   **Price already flows** via `formatPriceRange`
   ([context_assembly.go:210](../../../../../internal/workspace_knowledge/assembly/context_assembly.go#L210)).
   → We **upgrade** this layer; we do **not** rebuild it (`feedback_freeze_abstraction`).

2. **Wrong catalog content for the job.** The 131 rows are `AssetPODProduct`
   (physical goods). The screenshot lead wants a *fulfillment service*. A POD
   T-shirt cannot match a "need US fulfillment" lead, so the searcher returns
   **0 product hits** and the builder returns the base profile unchanged
   ([context_builder.go:182](../../../../../internal/workspace_knowledge/runtime/context_builder.go#L182)).
   The thing the comment must pitch (a **Service Offer**) is **not modeled**.

3. **Images dropped at assembly.** `renderProduct` reads origin/price/sizes/sku/
   availability/sourceURL — **never `pv.Images`**
   ([context_assembly.go:150-184](../../../../../internal/workspace_knowledge/assembly/context_assembly.go#L150)).

4. **Lossy, smuggled prompt contract.** The matched block is passed as the
   `businessContext` arg ([outbound_lead_pipeline.go](../../../../../cmd/scraper/outbound_lead_pipeline.go))
   landing under "BUSINESS PROFILE" ([msggen.go:130](../../../../../internal/ai/msggen.go#L130)),
   with only *"introduce your most relevant offering naturally"*
   ([msggen.go:142](../../../../../internal/ai/msggen.go#L142)). Output is prose, so outbound
   cannot extract an `image_url`, a price, or a confidence.

5. **Attribution gap.** Ledgers store `account_id`+`created_by`; `accounts` holds
   `fb_user_id/fb_display_name/fb_username/fb_profile_url`. But the engagement
   projection joins only `users`+`accounts.name`
   ([lead_engagement.go:267](../../../../../internal/store/leads/lead_engagement.go#L267)) and
   `CommentingView` renders bare `#account_id`
   ([CommentingView.tsx:311](../../../../../frontend/src/modules/autoflow/components/views/CommentingView.tsx#L311)).

6. **No Verified Actor.** The executor reads the live `c_user` into
   `NavDiagnostic.FBUserID` ([nav_diagnostic.go:68](../../../../../internal/models/nav_diagnostic.go#L68))
   but it lives only in `evidence_json` — **never compared** to the account's
   expected `fb_user_id`. Account #49 (`fb_user_id=111`) logged into `222` posts
   and is silently mis-attributed. **An autonomous system MUST NOT auto-execute
   without this check** — it is the single hardest precondition.

7. **Extension cannot attach media.** `executeComment` only `setEditableText` +
   submit ([outbound.js](../../../../../local-connector-extension/content/facebook/outbound/outbound.js)); no
   `uploadImage / waitPreview / submit`.

8. **"Anonymous participant" leaks into copy.** That label is the *target* (a FB
   anonymous group poster), captured verbatim
   ([crawl.js:166](../../../../../local-connector-extension/content/crawl.js#L166)); the prompt
   forces *"Address the author by their EXACT name"*
   ([msggen.go:139](../../../../../internal/ai/msggen.go#L139)).

---

## 1. Target pipeline — autonomous, end-to-end

Each stage is a **deterministic boundary** (branch on an explicit field, never a
proxy — `feedback_deterministic_boundaries`). The agent runs the whole chain
**without a human in the loop by default**; the Policy Gate is the only thing
that can divert to a human.

```
Lead (crawled post)
  │
  ▼
[1] Intent Classifier ──────────► intent + intent_confidence  (domain-agnostic enum)
  │
  ▼
[2] Grounded Selection ─────────► select from RETRIEVED knowledge only (asset_id/SKU + score)
  │     intent → which knowledge kinds dominate (no offer rows; raw assets only)
  ▼
[3] Comment Decision ───────────► CommentDecision (AGENT-OWNED, structured — NOT a draft):
  │       { intent, confidence, reasoning, knowledge_gap, retrieval_id,
  │         selected: { capabilities[], products[], proofs[], cta } }   ← each item CITED
  ▼
[4] (Media) ────────────────────► product/proof GroundedItem already carries a REAL ImageURL;
  │                                delivery is DEFERRED to P3 (no upload in P2)
  ▼
[5] Policy Gate ────────────────► verdict ∈ { auto_execute | require_review | reject }
  │       deterministic over: intent_confidence, offer_confidence, actor_verdict,
  │       media_realness, risk/cooldown/compliance flags, org auto-policy
  │
  ├── auto_execute ──► [6] Auto Execution ──► extension posts (P1–P2 text-only; P3 + media)
  ├── require_review ► held for operator override in the Agent Decision Inspector (FALLBACK)
  └── reject ───────► skip, typed reason recorded; no human needed
  │
  ▼
[7] Verified Actor ─────────────► expected fb_user_id (accounts) vs actual (NavDiagnostic)
        │                          verdict ∈ {verified, mismatch, unknown}
        ▼
   engagement_events / action_ledger  ← business truth (append-only; downstream reads HERE)
```

**This is not a CRM approval workflow.** There is no mandatory approval card.
`require_review` is the exception path (§6/§7), surfaced in an **inspector**, not
a queue the operator must clear before anything ships.

---

## 2. Knowledge as raw material — capabilities/offers/proofs are reasoning OUTPUTS

> **REVISED (rev 3).** The earlier "add `service_offer` / `proof_asset` /
> `case_study` authored types" plan is **withdrawn** — that was form-first. Per
> the founder directive, the agent SYNTHESIZES offers from raw knowledge; it does
> not read hand-authored offer rows.

### Raw material (already ingestable — no new connector, no CRUD)
`AssetType` today: `POD_product, faq, shipping_policy, sales_playbook,
pricing_rule, banned_claim, cta`
([assets/types.go:27](../../../../../internal/workspace_knowledge/assets/types.go#L27)), fed
by existing ingestors: `rest_json/shopify/csv/sheets` (catalog/SKUs),
`website/notion` (FAQ / playbooks / about / case studies as prose). The operator
connects **sources**; ingestion + the business profile
(`ai/profile_inference.go`, `ai/business.go`) are the raw material.

### How the founder's concepts map (output of reasoning, NOT stored types)
| Founder concept | Where it comes from at reason-time | NOT |
|---|---|---|
| **Capability / Offer** | named by the agent from retrieved `sales_playbook` / `faq` / `website` chunks + the business profile, each cited | a `service_offer` row |
| **Product SKU** | retrieved `POD_product` (catalog), with price + image already in payload | new type |
| **Proof / Case Study** | retrieved prose (case studies / testimonials from website/notion), cited | a `proof_asset` row |
| **Pricing** | `pricing_rule` assets and/or catalog price fields, cited | a pricing CRUD |
| **CTA** | retrieved `cta` asset, or synthesized from profile tone | unchanged |

**Reality check (drives graceful degradation §1):** in production today only the
catalog is reliably populated; `cta/pricing_rule/proof`-style assets exist mainly
in `soak/fixtures.go`. So capability/proof selections will be **empty until the
org connects website/Notion/FAQ sources** — the Reasoning Layer must degrade and
emit `knowledge_gap`, not fabricate.

### The ONLY storage this layer may add (agent-derived, not operator CRUD)
If runtime re-synthesis proves too costly/unstable, an **Extraction pass**
(agent, §9 P2b) may CACHE a derived business-understanding (capabilities, price
points, proof index) as **system-authored** assets carrying `provenance`
(source asset_ids). That is agent output with citations — reviewed, not typed
into a form. No human-CRUD-to-feed-the-agent is permitted.

---

## 3. Intent Classifier (Stage 1) — domain-agnostic

Decides which asset kind dominates selection so POD products are not forced into
a service lead. Emits an **explicit** `intent` + `intent_confidence ∈ [0,1]`.

| Intent | Meaning | Drives |
|---|---|---|
| `service_seeking` | wants a capability/done-for-you outcome | Service Offer + Proof + CTA |
| `product_seeking` | wants a specific item/SKU | Product SKU + CTA |
| `ambiguous` | unclear / broad | Service Offer + light Proof |
| `non_lead` | not an opportunity | **Policy Gate → reject** |

- **A (cheap, first):** rule + keyword/embedding score over lead text vs the
  org's own Service-Offer/Product tags — reuses the existing searcher signal, no
  new model call, deterministic + explainable, produces a calibrated confidence.
- **B (later):** small LLM classification when A is low-confidence.

`intent` + `intent_confidence` are recorded on the retrieval trace (extends the
existing replay trace).

---

## 4. Reasoning Layer (Stage 2) — grounded selection over raw knowledge

The Reasoning Layer is the heart of P2a. It takes a lead + the org's knowledge
and produces ONE explainable, GROUNDED decision. It **reuses**: `runtime.
BuildForLeadWithTrace` (retrieve candidates from KnowledgeOS + catalog),
`ai/universal.go UniversalClassify` (structured, profile-anchored reasoning) and
`ai/business.go BusinessProfile` (business understanding) — extended to emit the
founder's contract.

```go
// internal/ai (or internal/workspace_knowledge/runtime) — contract, not a DB row.
type CommentDecision struct {
    Intent      string             // service_seeking | product_seeking | ambiguous | non_lead
    Confidence  float64            // [0,1] composite the agent assigns to THIS decision
    Reasoning   string             // short human-readable WHY (for the inspector)
    Selected    Selection          // what the agent chose, each item GROUNDED
    KnowledgeGap bool              // true when knowledge was too thin to ground a real offer
    RetrievalID string             // joins to the replay trace (existing)
}

type Selection struct {
    Capabilities []GroundedItem   // named by the agent from retrieved playbooks/FAQ/site — CITED
    Products     []GroundedItem   // retrieved POD_product SKUs (price + image in payload)
    Proofs       []GroundedItem   // retrieved case-study / testimonial prose — CITED
    CTA          *GroundedItem    // retrieved cta asset, or profile-derived
}

// GroundedItem is the no-fabrication unit: a claim the agent makes MUST point at
// a real source. SourceAssetID>0 OR SKU!="" is REQUIRED; an item with neither is
// dropped (it would be an invented claim).
type GroundedItem struct {
    Label         string   // agent's phrasing ("US fulfillment from VN/CN, 3–5d")
    SourceAssetID int64    // the KnowledgeOS asset this is grounded in
    SKU           string   // catalog SKU when grounded in a product
    PriceText     string   // formatted; "" when unknown (nil-guarded)
    ImageURL      string   // from the cited asset; never generated
    Score         float64  // retrieval score / agent confidence for this item
}
```

**Grounding is enforced in code, not just prompted:** after the LLM returns its
selection, the layer **validates every GroundedItem against the retrieval hit set
by asset_id/SKU** and drops any the model invented. What survives is provably
backed by ingested knowledge. (This is why selection rides on top of retrieval,
not a free-form generation.)

**Reuse, not rebuild:**
- `BuildForLeadWithTrace` already returns scored hits + a replay trace → the
  grounding candidate set + `RetrievalID`.
- `renderProduct` **surfaces `Images[0]`** (the rev2 image fix still applies) so
  product `GroundedItem`s carry an image.
- `UniversalClassify` already does structured, profile-anchored LLM output →
  extend its schema to the `CommentDecision` shape instead of a new call path.

---

## 5. Comment text + Media (Stage 3/4) — generated from the grounded decision

The comment text is rendered from the §4 `CommentDecision` — the model writes
copy from the GROUNDED selection only, so it cannot pitch anything not backed by
ingested knowledge.

- **Prompt contract.** New `GenerateCommentV2(ctx, lead, decision, profile)`
  (keeps `GenerateCommentWithService` for staged removal). The prompt is built
  from `decision.Selected` (capabilities/products/proofs/cta with their labels +
  prices) + rules: anonymous-name guard (fixes cause #8), state a price ONLY when
  a `GroundedItem.PriceText` is present (nil-guard mandatory), pitch ≤1 capability
  + ≤1 proof, 2–3 sentences, no emojis, CTA from the chosen CTA item.
- **`KnowledgeGap == true` ⇒ no concrete offer.** The agent either writes a
  generic, honest reply (no invented specifics) or the row is held — the Policy
  Gate (§6) decides; it never fabricates to fill the gap.

### Media — DEFERRED (founder: "chưa cần media upload" for P2)
Media selection/decision is **out of scope for P2a**. A product/proof
`GroundedItem` already carries `ImageURL` (a real asset), so when P3 adds the
extension upload capability the image candidate is already chosen and grounded.
No media is uploaded in P2; no AI image is ever generated.

---

## 6. Policy Gate (Stage 5) — deterministic auto/​review/​reject

Replaces "Approval." Pure function of explicit fields; emits one typed verdict +
a typed reason code. **No mandatory human step.**

```go
type GateVerdict struct {
    Decision string      // auto_execute | require_review | reject
    Reason   ReasonCode  // typed: why this route
}
```

**`auto_execute` iff ALL hold (founder requirement #3):**
1. `intent != non_lead` and `intent_confidence ≥ org.min_intent_confidence`
2. `offer_confidence ≥ org.min_offer_confidence` (or org allows generic comments)
3. `actor_verdict == verified` (expected fb_user_id == actual — §7)
4. every concrete claim is a surviving `GroundedItem` (cites a real asset_id/SKU)
   and `knowledge_gap == false` — nothing fabricated; any `ImageURL` is a real
   ingested asset, never generated
5. no blocking `RiskFlags` (cooldown active, compliance warning, duplicate target)
6. `org.auto_execute_enabled == true` (the opt-in; default off — Hard-Rule §⚠️)

**`require_review` (FALLBACK, founder requirement #4)** when any of:
low confidence (intent or offer), ambiguous offer, `actor_verdict ∈
{unknown,mismatch}`, media missing/risky, a compliance warning flag, or the org
explicitly runs in review mode.

**`reject`** for `non_lead`, hard compliance violation (banned claim), or a hard
cooldown/duplicate — recorded with a typed reason; **no human needed**.

The gate is the *only* component that can route to a human, and it does so by
**exception**, preserving the safety floor the Hard Rule intends while defaulting
to autonomy when the org opts in.

---

## 7. Verified Actor (Stage 7) + attribution — **P1, highest priority**

Data exists; wiring does not. This is also auto-execute precondition #3, so it
is the gating dependency for autonomy.

### 7a. Surface the actor (display)
- Extend the engagement projection to join `accounts.fb_display_name` +
  `accounts.fb_profile_url` (today only `accounts.name` —
  [lead_engagement.go:267](../../../../../internal/store/leads/lead_engagement.go#L267)).
- Two identities, always distinct (`feedback_outbound_taxonomy_split`):
  **Initiator** = `created_by`→staff (who/what ordered it; for autonomous runs a
  system/agent principal); **Executor (FB actor)** = `account_id`→
  `accounts.fb_display_name` + profile URL.

### 7b. Verified Actor (integrity)
- **Expected:** `accounts.fb_user_id` for the assigned `account_id`.
- **Actual:** `NavDiagnostic.FBUserID` (`c_user`) at execution
  ([nav_diagnostic.go:68](../../../../../internal/models/nav_diagnostic.go#L68)).
- At finalize, compute `actor_verdict ∈ {verified, mismatch, unknown}` and emit
  it on the **engagement event** (ledger = truth, append-only —
  `feedback_verified_state_centric`, `feedback_append_only_correction_events`).
- **Pre-execution check for autonomy:** auto_execute requires
  `actor_verdict==verified`. Because the executor only reads `c_user` *during* a
  run, the gate uses the **last known verified identity** for the account; a
  mismatch detected at finalize emits a correction event, **blocks the account**
  from further auto-execute, and raises an inspector alert.

### Schema (additive)
- `engagement_events` / `action_ledger` emit `expected_fb_user_id`,
  `actual_fb_user_id`, `actor_verdict`. Migration additive; old rows = `unknown`.

---

## 8. UI — **Agent Decision Inspector** (not an approval card)

The primary surface is observational + override, not a gate the operator must
clear (founder requirement #5).

- **Per decision, show:**
  - Intent + `intent_confidence`; **why** (typed reason codes rendered human).
  - Selected Offer + **why this offer** (matched tags/score), `offer_confidence`.
  - Selected Media thumbnail + **why this image** + its `Status`
    (`selected_pending_delivery_capability` in P1–P2).
  - Composite `Confidence` and `RiskFlags`.
  - **Gate verdict**: `auto_execute` (already posted ✅) / `require_review`
    (held ⏸) / `reject` (skipped ⛔) + the typed reason.
  - **Executor**: FB display name + profile URL + Account #; **Verified Actor**
    chip (✅ verified / ⚠️ mismatch expected 111 actual 222 / ❔ unknown).
- **Override controls** (the fallback, used by exception): change offer, change
  media, edit text, force-execute, or veto — each override is itself recorded on
  the ledger as an event with the operator principal.
- **"Anonymous participant"** rendered "(người đăng ẩn danh)"; no salutation in copy.
- **Product Explorer:** badges for `service_offer / proof_asset / case_study`.

> The inspector answers "why did the agent do this, and was it right?" — the
> Organic Sales Network war-room posture, not a CRM inbox of pending approvals.

---

## 9. Phasing (staged evolution — additive PRs, no big bang)

### P1 — Attribution + Verified Actor *(no media, no offer changes)*
- **P1a** projection join + dual-identity display (cheap, immediate; unblocks the
  inspector's Executor + Verified-Actor chip).
- **P1b** Verified Actor: promote `FBUserID`, add `expected/actual/verdict`
  columns to the engagement event, finalize-time compare, account auto-block on
  mismatch. *Precondition for any auto-execute.*

### P2 — Knowledge Intelligence Layer *(knowledge-first; text-only execution)*
No CRUD. Reuses KnowledgeOS retrieval, catalog sync, CTA assets, `UniversalClassify`,
`profile_inference`, and P1 attribution/verified-actor.
- **P2a** **Reasoning Layer**: lead → intent + GROUNDED selection over EXISTING
  KnowledgeOS + catalog → `CommentDecision` (§4) with citations, confidence,
  `knowledge_gap`. Reuses `BuildForLeadWithTrace` + extends `UniversalClassify`;
  enforces grounding in code (drop invented items); surfaces `Images[0]`. Pure
  reasoning — buildable now, degrades gracefully on thin knowledge.
- **P2b** (PARTIAL, SHIPPED): the `csv` ingestor is registered + an admin,
  tenant-scoped endpoint `POST /api/knowledge/seed-service` (idempotent; creates
  one csv source → syncs → optionally approves) lets operators seed RAW service
  knowledge without DevTools/CRUD. Domain data lives in `scripts/seed_service_
  knowledge.*`, never in the binary. `website/notion` ingestors remain stubs
  (a real crawler is future work). Full P2b also wants to:
  make `website/notion` (and a pricing/FAQ/case-study text source) reliably populate non-product knowledge,
  and optionally CACHE a derived business-understanding (agent-authored +
  provenance — NOT a form) so per-lead reasoning is cheap and capability/proof
  selections are non-empty. Closes the §1 graceful-degradation gap with real data.
- **P2c-dry-run** (SHIPPED): compute `CandidatesForLead` + `DecideComment` in the
  queue path and **persist the decision for observation only** — comment text and
  execution unchanged. Gated by `THG_COMMENT_REASONING_DRYRUN=1` so the live path
  pays nothing unless validating. Lets us check reasoning on real data before it
  drives output.
- **P2c** (SHIPPED, behind env): `GenerateCommentV2` writes copy from the grounded
  decision only (anonymous-name + price nil-guards). A single hot kill-switch
  `THG_COMMENT_REASONING=off|dryrun|live` (default `off`): `live` lets a GROUNDED
  decision drive the comment text; `knowledge_gap` falls back to the existing
  generic generation (NO regression); execution/auto-policy unchanged. Founder
  flips it to `live` on prod to test; flips back to `off` to disable without redeploy.
- **P2d** **Policy Gate** (auto/review/reject) before execution, default
  `org.auto_execute_enabled=false`; typed reason codes; enforces §12 invariant
  (verified-only auto) AND grounding (no auto-execute on `knowledge_gap` /
  ungrounded claim).
- **P2e** **Agent Decision Inspector** UI: intent, `reasoning`, each grounded
  selection **with its source citation**, confidence, `knowledge_gap` nudge,
  gate verdict, executor + verified-actor chip. Observe + override.
- *Outcome:* the agent reasons over the org's own knowledge and **auto-executes
  grounded text comments** end-to-end for opted-in orgs; every claim is cited and
  inspectable; nothing is fabricated.

### P3 — Auto media delivery behind the Policy Gate *(high-risk, NOT blanket-approval)*
- Extension `executeComment`: `uploadImage → waitPreview → submit`, forensics-first
  (per the archived root-cause report discipline, ../comment-automation/evidence/root-cause-report.md), image rotation to avoid spam signatures.
- **Risk-based delivery (founder requirement #7):** the same Policy Gate decides
  media delivery — `safe + high confidence → auto`; `risky/low confidence →
  require_review`. No global "every image needs approval."

### P4 — Vision matching
- Read the post's own images (already in `posts.images`), describe → match to
  Service Offer / Product SKU. Only after P2 proves offer value.

---

## 10. Invariants honored

- **Autonomous-by-default, gated by safety:** the Policy Gate, not a human, is the
  default arbiter; review is the exception. Hard-Rule reconciliation in §⚠️.
- **Typed reason codes everywhere** (`Reason`, `RiskFlag`, `GateVerdict.Reason`,
  `actor_verdict`): the gate and inspector branch on closed enums, never prose
  (`feedback_deterministic_boundaries`).
- **No AI-generated images:** a `GroundedItem.ImageURL` always resolves to a real
  ingested catalog/proof asset; the gate rejects auto-execute otherwise.
- **Verified actor before autonomy:** auto_execute requires `actor_verdict==verified`.
- **Append-only ledger:** verified-actor fields + override events are emitted, never
  UPDATEs; mismatches reconcile via correction events.
- **Verified-state-centric:** UI/automation read the engagement ledger for actor +
  outcome, never `outbound_messages.status`.
- **Contracts not ORM:** `OfferSelection / CommentDecision / GateVerdict` are
  domain contracts.
- **Tenant isolation:** every asset kind, projection, ledger column, and org-policy
  carries `org_id`; cross-domain reads `// tenant-ok`, writes via Hooks.
- **Domain-agnostic:** intent enum + Service-Offer schema generic; THG's offers are data.
- **Observable automation:** execution stays in the visible extension; every
  autonomous action is inspectable after the fact.
- **Not a CRM:** no mandatory approval queue; the Agent Decision Inspector is a
  war-room observ/override surface (`feedback_shared_battlefield_not_crm`).
- **No "coordination service" naming:** domain nouns (`OfferSelection`,
  `IntentClassifier`, `PolicyGate`), not Engine/Manager/Service/Dispatcher/Coordinator.

---

## 11. Open questions (resolve before P2e/P3)

1. ~~**Offer authoring UX:** bulk CSV/JSON import vs a form.~~ **WITHDRAWN (rev 3):**
   no offer authoring at all — offers are agent reasoning outputs from raw
   knowledge sources (§2). The open question becomes: which raw sources does THG
   connect first (website? a pricing/FAQ doc?) so capability/proof selections are
   non-empty (P2b).
2. **Confidence thresholds:** `org.min_intent_confidence` / `min_offer_confidence`
   starting values + calibration method.
3. **Hard-Rule amendment:** founder to confirm the `CLAUDE.md` wording change in §⚠️
   so `auto_execute_enabled=true` on the OSN org is rule-compliant, not an override.
4. **Proof Asset storage:** reuse the existing uploaded-file store or a new bucket.
5. **Autonomous principal identity:** what `created_by` records for an agent-initiated
   action (a reserved system/agent user id) so the ledger's Initiator axis stays valid.
6. **Mismatch blast radius:** on `actor_verdict==mismatch`, block just the account or
   the whole org's auto-execute until an operator clears it.
   → **DECIDED (founder, 2026-06-08): block the account only + alert.** Org-level
   escalation is deferred until repeated mismatches across multiple accounts.
7. **P1b verdict atomicity (tech debt, tracked):** `MarkAttemptActorVerification` +
   `RecordAccountActorVerdict` run AFTER `FinishExecutionAttempt`, not in the same
   transaction. A crash between them finalizes the attempt with no verdict/block.
   Harden by folding all three into one coordination finalize transaction. Code
   carries `TODO(p1b-atomicity)` at the call site (outbox_agent.go).
8. **Org-level mismatch escalation (follow-up):** count consecutive mismatches across
   accounts → raise an org incident (vs the current per-account block). Verdict data
   is already recorded to compute this later.

---

## 12. BINDING invariant for P2e (Policy Gate) — Verified Actor

When the Policy Gate ships, this is **non-negotiable** and must be covered by a test:

- `auto_execute` is permitted **only** when `actor_verdict == verified`.
- `actor_verdict == unknown` (could not read `c_user`) → **never auto_execute** →
  route to `require_review`. An account whose live identity cannot be confirmed must
  not act autonomously.
- `actor_verdict == mismatch` → account blocked (P1b) → the Policy Gate also rejects.

P1b enforces the mismatch block today; P2e must enforce the `verified`-only rule for
the *positive* auto-execute decision so an `unknown` account never slips through.

### BINDING grounding invariant (rev 3)
- A comment may auto-execute **only** when every concrete claim in it is a
  surviving `GroundedItem` (cites a real `asset_id`/SKU). `knowledge_gap == true`
  or any ungrounded selection → **never auto_execute** → `require_review` or a
  generic non-specific reply. The agent never invents an offer, price, or proof.
- This is the knowledge analog of P1b's "real asset" rule and must be covered by
  a Policy-Gate test alongside the verified-actor test.

### P1b follow-up status (2026-06-08, shipped)
- ✅ Operator override: admin `POST /accounts/:id/clear-actor-block` + FE "Gỡ chặn
  actor" button (shown on a blocked account) + list-row blocked indicator.
- ✅ Operator alert: `NotifyActorMismatch` routes a problem event to the account
  owner's chat on block, plus the high-signal `slog.Error`.
- ◻️ Atomicity hardening (§11.7) — open.
- ◻️ Org-level escalation (§11.8) — open.
```

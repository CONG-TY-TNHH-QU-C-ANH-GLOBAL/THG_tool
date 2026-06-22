# Lead Ingestion Behavior Specification (Characterization Snapshot)

> **Purpose.** This document is a *descriptive snapshot* of the **current** behavior of
> `internal/leadingest/ingest.go` as it exists at the time of writing (main `d41162eb`,
> the PR22B merge). It exists to anchor a characterization harness so PR23C can reduce
> the cognitive complexity of `IngestPost` (`go:S3776`, complexity ~105) **without
> changing behavior**.
>
> It is **not** a product requirements document and does **not** describe desired future
> behavior. Where the code is ambiguous it says `Unclear from current code`. Where a
> behavior cannot be observed without a production seam it is listed under
> **Untestable Gaps**. Current quirks are called out explicitly even when they look odd.

---

## 1. Scope

* **Target file:** `internal/leadingest/ingest.go`
* **Target function (refactor target):** `IngestPost(ctx context.Context, deps Deps, in Input) (Outcome, error)` — lines 223–541.
* **Related files inspected (current code):**
  * `internal/leadingest/force_lead.go` — `Deps.overrideVeto` + `ForcedLeadCategory`.
  * `internal/store/app_store.go` — `AppStore.InsertLead`, `task_leads` schema (`UNIQUE(task_id, source_url)`), `GetLeadCounts`, `ListLeads`.
  * `internal/store/leads/leads.go` — legacy `Store.InsertLead` (`INSERT OR IGNORE INTO leads ...`).
  * `internal/store/migrations/0001_legacy_baseline__sqlite.up.sql` — `CREATE UNIQUE INDEX idx_leads_dedup ON leads(source_type, source_id) WHERE source_id > 0`.
  * `internal/ai/msggen.go` (`MessageGenerator.Available`) + `internal/ai/universal.go` (`UniversalClassify`).
  * Existing tests: `ingest_test.go`, `force_lead_test.go`, `routing_test.go`.
* **What this spec covers:** the synchronous classify → persist → side-effect flow of `IngestPost` and its private helpers (`repairPrimaryURL`, `buildURLRepairSignal`, `matchAny`, `normalizeSourceType`, `ValidateRouting`, `Deps.overrideVeto`).
* **What this spec does NOT cover:** the AI classifier internals (`ai.MessageGenerator`), the scorer internals (`scoring.Scorer`), the store SQL internals beyond the two `INSERT` statements named above, the callers (`internal/jobhandlers/facebook_crawl`, `internal/server/agent/crawl.go`), and `SignalGateFromMap`/`str`/`f64`/`strSlice` (already covered by `ingest_test.go`).

---

## 2. Entry Points

### `IngestPost(ctx, deps Deps, in Input) (Outcome, error)`

* **Input type:** `Deps` (per-run dependencies; all store/AI fields are nil-tolerant) + `Input` (one crawled candidate).
* **Output type:** `(Outcome, error)`.
* **Success return behavior:** returns an `Outcome` describing the classification and, when it reaches the persist block, sets `Outcome.Inserted = true`. `error` is non-nil **only** when `AppStore.InsertLead` fails (the single fatal path — see §8).
* **Failure return behavior:** every *classification* rejection (blank content, invalid routing, deterministic reject, gate reject, AI reject, cold) returns `(Outcome{Skipped: <reason>}, nil)` — i.e. **not** a Go `error`. The reason is carried in `Outcome.Skipped`.
* **Side effects:** see §7. None occur unless the corresponding `Deps` field is non-nil and the input is not skipped before the persist block.

### Helper functions called by the main flow (all package-private unless noted)

* `repairPrimaryURL(in *Input) bool` — mutates `in.PrimaryURL`/`in.PostFBID` in place; returns whether `PrimaryURL` was rewritten.
* `ValidateRouting(in Input) error` (exported) — routing contract gate.
* `normalizeSourceType(s string) string` — maps to `"post"`|`"comment"`.
* `buildURLRepairSignal(crawlerPath string, pipelineRepaired bool) string` — builds the `url:<path>` telemetry signal.
* `matchAny(content string, phrases []string) string` — first case-insensitive substring match; `""` if none.
* `Deps.overrideVeto(out *Outcome, verdict string) bool` — direct-post (`ForceLead`) veto downgrade (see §5/§9).

---

## 3. Input and Payload Contract

`IngestPost` does **no** JSON/body decoding — it consumes an already-decoded `Input` struct. (`SignalGateFromMap` decodes the gate map, but is not on the `IngestPost` path.)

* **Content (`in.Content`):** trimmed (`strings.TrimSpace`). **Empty trimmed content → `Outcome{Skipped: "filter"}`, no error, no side effects** (ingest.go:224–227). This is the first gate.
* **`in.SourceType`:** normalized via `normalizeSourceType`: `"comment"` (case-insensitive) → `"comment"`; everything else (incl. empty/unknown) → `"post"`.
* **`in.PrimaryURL`:** the canonical POST url; required by `ValidateRouting` (see §5/§9). May be repaired in place first (see below).
* **`in.SecondaryURL`:** optional comment url; only consulted by `ValidateRouting` when `SourceType == "comment"`.
* **`in.PostFBID` / `in.GroupFBID`:** used by `repairPrimaryURL` to synthesize a canonical permalink, and by the cursor-advance path.
* **`in.URLRepairPath`:** crawler telemetry string, surfaced as `url:<path>` (see §7 / `buildURLRepairSignal`).
* **Default values:** scorer defaults to `scoring.New(scoring.DefaultConfig())` when `deps.Scorer == nil` (ingest.go:257–260); `ClassifyTimeout` defaults to `20 * time.Second` when `<= 0` (ingest.go:304–307).
* **Tenant/org requirements:** `in.OrgID` is carried through to persisted rows and `LeadEvent`, but `IngestPost` itself performs **no** org-ownership check — it trusts the caller. `Unclear from current code` whether any caller enforces it before calling (out of scope).
* **DTOs / JSON tags:** unchanged by this spec. PR23C must not change them.

---

## 4. DB Interactions

All DB work is gated on the corresponding non-nil `Deps` field. With `AppStore == nil` **and** `LegacyDB == nil`, `IngestPost` performs **zero** DB I/O yet still computes the full `Outcome` and sets `Inserted = true` at the end (ingest.go:521). This is the property the deterministic harness relies on.

| Table | Operation | When | Key / dedup | Failure behavior |
| --- | --- | --- | --- | --- |
| `task_leads` | `INSERT OR IGNORE` (`AppStore.InsertLead`, app_store.go:312) | `deps.AppStore != nil` and not skipped before persist | `UNIQUE(task_id, source_url)` (app_store.go:90) | **FATAL** — `IngestPost` returns `(out, err)` (ingest.go:433–435). (`INSERT OR IGNORE` does *not* error on duplicate, so this fires only on a real DB error.) |
| `leads` (legacy) | `INSERT OR IGNORE` (`leads.Store.InsertLead`, leads.go:103) | `deps.LegacyDB != nil` and not skipped before persist | partial `UNIQUE idx_leads_dedup(source_type, source_id) WHERE source_id > 0` | **Best-effort** — error logged (`legacy lead mirror failed`), flow continues (ingest.go:486–492) |
| `classification_log` | `INSERT` via `Leads().RecordClassification` | only inside the AI block (`deps.AIClass` available), once per AI outcome | n/a | Best-effort — error ignored (`_ =`) (ingest.go:344, 372, 400) |
| `conversation_threads` | `INSERT OR IGNORE` via `Threads().SeedThreadForOrg` | `deps.LegacyDB != nil`, persist reached, and `in.AuthorProfileURL != ""` | `idx_thread_org_profile` (idempotent) | Best-effort — error logged, flow continues (ingest.go:501–506) |
| crawl intent cursor | `Crawl().AdvanceIntentCursor` | `deps.IntentID > 0 && deps.LegacyDB != nil`, persist reached, and a non-empty post id is resolvable | per-intent cursor | Best-effort — error logged, flow continues (ingest.go:527–538) |

---

## 5. Deduplication / Idempotency Logic

`IngestPost` does **no** application-level dedup pre-check; it always calls the two `INSERT OR IGNORE` statements. Dedup is therefore enforced entirely by the DB unique constraints.

* **`task_leads` dedup key:** `UNIQUE(task_id, source_url)`. A second ingest with the **same `TaskID` and same `PrimaryURL`** is silently ignored (no new row, **no error**) — the lead is *treated as success* (flow continues, `Inserted` still set). Dedup is scoped by **`task_id` + `source_url`**, **not** by `org_id`. **Quirk:** because the key is `(task_id, source_url)` and `task_id` is the crawl-run id, the same URL ingested under a *different* `TaskID` produces a **second** `task_leads` row.
* **Legacy `leads` dedup:** the only unique index is partial: `idx_leads_dedup(source_type, source_id) WHERE source_id > 0`. **`IngestPost` always sets `SourceID: 0`** (ingest.go:467). Therefore the partial index **never applies to ingest-path leads**, and **every** `IngestPost` call that reaches the persist block inserts a **new** `leads` row — **the legacy mirror is not deduplicated for this path.** This is a current quirk, documented here so PR23C preserves it.
* **Behavior on repeated ingestion (same TaskID+URL, both stores set):** `task_leads` → 1 row; `leads` → N rows (one per call); `OnLeadCreated` → fired once **per call** (see §7). No error returned.
* **Behavior on partial match / missing dedupe fields:** `Unclear from current code` beyond the two constraints above; `IngestPost` does not branch on partial matches.

---

## 6. Scoring / AI Logic

### Deterministic scoring (always runs, testable)

* Triggered unconditionally for non-empty, routable content via `scorer.ScoreWithGuidance(...)` (ingest.go:261). Pure Go keyword scoring — **no external call**, deterministic.
* `sr.Category == "rejected"` → `Outcome.Skipped = "rejected"`; returns early **unless** `ForceLead` overrides (ingest.go:274–279).
* `Outcome.Score`, `Outcome.Category`, `Outcome.Signals` are seeded from the scorer result.

### Market-signal gate (deterministic, string matching)

* `matchAny(content, SignalGate.RejectRules)` hit → `Skipped="gate_negative"`, `Category="rejected"`, signal `gate_reject:<phrase>`; early-return unless `ForceLead` overrides (ingest.go:283–290).
* `matchAny(content, SignalGate.NegativeSignals)` hit → `Skipped="gate_negative"`, `Category="rejected"`, signal `gate_negative:<phrase>`; early-return unless `ForceLead` overrides (ingest.go:291–298).
* **Quirk:** both branches set `Skipped="gate_negative"`, but the *signal prefix differs* (`gate_reject:` vs `gate_negative:`). The `MinConfidence` / `PositiveSignals` fields are **not** consulted as gate logic in `IngestPost` (PositiveSignals is only forwarded to the AI classifier intent).

### AI classifier (best-effort, **NOT testable without a production seam**)

* Runs only when `hasAIContext && deps.AIClass != nil && deps.AIClass.Available()` where `hasAIContext = (BusinessProfile configured) || (UserPrompt non-empty)` (ingest.go:302–303).
* `MessageGenerator.Available()` returns `apiKey != ""` (msggen.go:101); `UniversalClassify` performs an **external HTTP call** (universal.go:174). There is no interface seam — `Deps.AIClass` is a concrete `*ai.MessageGenerator`.
* **Best-effort:** on classifier error, it logs and **falls back to the deterministic result** (a flaky LLM never blocks capture) (ingest.go:338–345).
* On success it may hard-reject (target-role guard, ingest.go:347–359), set `Skipped/Category="rejected"` with `ai_intent:`/`ai_reason:` signals, or overwrite `Score = aiResult.Score * 100` and `Category = aiResult.Priority` (defaulting empty → `"cold"`).
* Writes a `classification_log` row for kept / cold / rejected / errored outcomes (best-effort).
* **No retry/backoff** beyond the single `context.WithTimeout`.
* **Conclusion:** the AI branch is an **Untestable Gap** for a 0-production-file harness (see §11).

### Cold gate

* After scoring/AI, `Outcome.Category == "cold"` → `Skipped="cold"`; early-return unless `ForceLead` overrides (ingest.go:406–411).

---

## 7. Side Effects

| Side effect | Trigger | Sync/async | Fatal? | Failure behavior | Deterministically testable now? |
| --- | --- | --- | --- | --- | --- |
| `task_leads` insert | `AppStore != nil`, persist reached | sync | **Fatal** (returns err) | propagates error | **Yes** (real store) |
| `leads` insert | `LegacyDB != nil`, persist reached | sync | Best-effort | logs `legacy lead mirror failed` | **Yes** (real store) |
| `conversation_threads` seed | `LegacyDB != nil`, persist reached, `AuthorProfileURL != ""` | sync | Best-effort | logs `thread seed failed` | Partially (row not asserted here) |
| `OnLeadCreated(LeadEvent)` notification hook | `LegacyDB != nil`, persist reached, `OnLeadCreated != nil` | sync (caller-defined func; doc says must not block) | Best-effort | n/a — return value ignored | **Yes** (capture via a test callback) |
| `classification_log` insert | AI block only | sync | Best-effort | error ignored | No (AI gap) |
| crawl intent cursor advance | `IntentID > 0 && LegacyDB != nil`, persist reached, post id resolvable | sync | Best-effort | logs `advance crawl intent cursor failed` | Partially (not asserted here) |
| `slog` warn logs | various best-effort failures | sync | n/a | n/a | Not asserted (log capture out of scope) |

**Notes.**
* `OnLeadCreated` is an existing **field of `Deps`** (a `func(LeadEvent)`), **not** a production interface or DI added for tests. Passing a closure that records the events is the documented, in-contract way to observe it. It fires **once per `IngestPost` call that reaches the persist block** — so a duplicate (task-deduped) ingest still fires the hook a second time. `LeadEvent.Excerpt` is the **raw** trimmed content (the consumer sanitizes downstream). `LeadEvent.Reason` is `FirstNonEmpty(AIReason, join(Signals, " / "))`.
* `IngestPost` spawns **no goroutines, channels, or timers** itself (the only `context.WithTimeout` is in the AI block and is cancelled synchronously). There is nothing async to wait on.

---

## 8. Error Handling

* **Only fatal path:** `AppStore.InsertLead` failure → `return out, err` (ingest.go:433–435). Everything else downstream is best-effort.
* **Best-effort / logged-only:** legacy `leads` insert, thread seed, cursor advance, `classification_log` writes — all log (or ignore) and continue.
* **Ignored errors:** `classification_log` writes use `_ =`; `OnLeadCreated` has no return value.
* **Classification "failures" are not Go errors:** blank/invalid/reject/gate/cold all return `(Outcome{Skipped:...}, nil)`.
* **Sentinel/typed errors:** `ValidateRouting` returns plain `errors.New(...)` strings (no sentinels); the message text is embedded into the `invalid_routing:<msg>` signal. There are **no** `errors.Is`/`errors.As` checks in `IngestPost`, so none must be introduced or removed by PR23C. The `invalid_routing:` signal text is part of the observable contract.
* **Transaction behavior:** none — `IngestPost` issues independent statements; there is no surrounding transaction or rollback.

---

## 9. Current State Machine

Confirmed flow of `IngestPost` (only states present in code):

```text
1.  Trim content; if empty -> return Outcome{Skipped:"filter"}.                (224)
2.  repairPrimaryURL(&in): if PrimaryURL is a shell but PostFBID/ID resolvable,
    synthesize a canonical permalink in place; record whether it mutated.      (234)
3.  Build urlSignal from (URLRepairPath, pipelineRepaired).                     (240)
4.  ValidateRouting(in); on error -> return Outcome{Skipped:"invalid_routing",
    Signals:["invalid_routing:<msg>", maybe urlSignal]}.                       (245)
5.  normalizeSourceType(in.SourceType).                                        (255)
6.  Deterministic score (default scorer if nil); seed Score/Category/Signals;
    append urlSignal + Deps.ExtraSignals.                                      (257)
7.  If Category=="rejected" -> Skipped="rejected"; return unless ForceLead.    (274)
8.  Gate RejectRules match -> Skipped="gate_negative", Category="rejected",
    signal gate_reject:<p>; return unless ForceLead.                           (283)
9.  Gate NegativeSignals match -> Skipped="gate_negative", Category="rejected",
    signal gate_negative:<p>; return unless ForceLead.                         (291)
10. If hasAIContext && AIClass.Available(): classify (best-effort), maybe
    reject / overwrite Score+Category, write classification_log.               (300)
11. If Category=="cold" -> Skipped="cold"; return unless ForceLead.            (406)
12. threadRole = InferThreadRole(sourceType, AIIntent, content).               (418)
13. If AppStore!=nil: InsertLead(task_leads); on error -> return (out, err).    (420)
14. If LegacyDB!=nil: InsertLead(leads, SourceID=0); thread seed (if profile);
    OnLeadCreated hook (if set).                                               (437)
15. out.Inserted = true.                                                       (521)
16. If IntentID>0 && LegacyDB!=nil: advance crawl cursor (best-effort).        (527)
17. return out, nil.                                                           (540)
```

`Deps.overrideVeto` (force_lead.go:26): when `ForceLead` is true it appends
`market_filter_result:<verdict>`, `filter_override_applied:true`,
`explicit_user_requested:true`, clears `Skipped`, and promotes a
`rejected`/empty category to `ForcedLeadCategory = "warm"`, returning `true` so
steps 7/8/9/11 fall through to persist instead of returning.

---

## 10. Characterization Test Matrix

Legend: **(existing)** already pinned by `ingest_test.go` / `force_lead_test.go` / `routing_test.go`; **(new)** added by this PR.

| Behavior | Testable now? | Test name | Notes |
| --- | ---: | --- | --- |
| Blank content → `filter` | Yes (existing) | `TestIngestPost_BlankContentSkipped` | already pinned |
| Deterministic hot/warm qualifies (no AI) | Yes (existing) | `TestIngestPost_DeterministicHotLeadQualifies` | already pinned |
| Cold → `cold`, not inserted | Yes (existing) | `TestIngestPost_ColdLeadIsSkippedNotInserted` | already pinned |
| Gate `RejectRules` → `gate_reject:` | Yes (existing) | `TestIngestPost_RejectRuleHardRejects` | already pinned |
| `ValidateRouting` sub-rules | Yes (existing) | `TestValidateRouting` | direct, not via `IngestPost` |
| `repairPrimaryURL` mutation | Yes (existing) | `TestRepairPrimaryURL` | helper-level |
| ForceLead overrides gate-reject + cold | Yes (existing) | `TestIngestPost_ForceLead*` | RejectRules + cold branches |
| **Invalid routing mapped to `Outcome` (via `IngestPost`)** | Yes (new) | `TestIngestPost_InvalidRoutingMapsToOutcome` | pins `Skipped="invalid_routing"` + `invalid_routing:<msg>` + url signal, `Inserted=false` |
| **Gate `NegativeSignals` → `gate_negative:` + ForceLead override** | Yes (new) | `TestIngestPost_NegativeGateRejectsAndForceLeadOverrides` | one fixture: distinct prefix from `gate_reject:`, and override → "warm" lead (completes the override matrix) |
| **`ExtraSignals` appended to `Outcome.Signals`** | Yes (new) | `TestIngestPost_ExtraSignalsAppended` | pins the connector-tag passthrough |
| **URL repair signal surfaces through `Outcome`** | Yes (new) | `TestIngestPost_URLRepairSignalSurfaces` | passthrough `url:<path>` and in-pipeline `url:repaired_in_pipeline` |
| **`buildURLRepairSignal` precedence** | Yes (new) | `TestBuildURLRepairSignal` | pipeline-repaired wins over crawler path |
| **Happy-path persistence + notify once** | Yes (new, DB) | `TestIngestPost_PersistsLeadAndNotifiesOnce` | fresh `store.New(t.TempDir())`; asserts `task_leads` count=1, persisted fields, 1 `LeadEvent` |
| **`task_leads` idempotency by (task_id, source_url) + notify per call** | Yes (new, DB) | `TestIngestPost_TaskLeadDedupByTaskAndURL` | same input twice → 1 row, hook fires twice; different `TaskID` → 2 rows |
| AI classifier kept/rejected/override | **No** | — | Untestable Gap §11 |
| `classification_log` rows | **No** | — | Untestable Gap §11 (AI-gated) |
| Thread seed row contents | Not asserted | — | best-effort; row existence not pinned here |
| Crawl cursor advance | Not asserted | — | best-effort; requires intent fixture |
| `slog` warn outputs | Not asserted | — | log capture out of scope |

---

## 11. Untestable Gaps

These behaviors are **not** pinned by this harness, by design, because observing them would require a production seam / external dependency this PR must not add:

1. **AI classifier path** (`deps.AIClass`): `*ai.MessageGenerator` is a concrete type whose `Available()` needs a non-empty API key and whose `UniversalClassify` makes an external HTTP LLM call. There is no interface to fake. Adding a `Classifier` interface / DI is an explicit non-goal → **gap**.
2. **`classification_log` writes**: only reachable inside the AI block → **gap** (follows from #1).
3. **Target-role hard-reject guard** (ingest.go:347–359): AI-gated → **gap**.
4. **Thread-seed row assertions & crawl-cursor advance**: reachable with a real store, but asserting their persisted state needs intent/thread fixtures beyond the lead path; left as best-effort (their *non-fatal* nature is documented, not asserted).
5. **`slog` log emission**: log capture is out of scope; the best-effort/logged classification is documented but not asserted.
6. **Caller-side org-ownership enforcement**: `IngestPost` trusts `in.OrgID`; whether callers check it is out of scope.

Gaps are acceptable and explicitly recorded so PR23C does not assume they are covered.

---

## 12. PR23C Refactor Constraints

PR23C (complexity reduction of `IngestPost`) **must**:

* Keep every test referenced in §10 green (this is the safety net).
* Preserve the §9 state machine ordering and every observable `Outcome.Skipped` value
  (`"" | filter | invalid_routing | rejected | gate_negative | cold`) and signal string
  (`invalid_routing:<msg>`, `gate_reject:<p>`, `gate_negative:<p>`, `url:<path>`,
  `url:repaired_in_pipeline`, `ai_intent:`, `ai_reason:`, and the three `overrideVeto`
  annotations).
* Preserve dedup behavior exactly: `task_leads` `UNIQUE(task_id, source_url)`; legacy
  `leads` with `SourceID=0` (no dedup). **Do not** add a dedup pre-check or change
  `SourceID`.
* Preserve side-effect behavior: `task_leads` insert fatal; `leads` insert / thread seed /
  cursor advance / `OnLeadCreated` best-effort; hook fires once per non-skipped call.
* **Not** change: DTO fields, JSON tags, status/category strings, DB schema, migrations,
  the scoring trigger, the AI trigger condition, the notification trigger condition, queue
  dispatch, or tenant-isolation behavior.
* **Not** introduce: shared/common util packages, helper soup, new production DI/interfaces,
  or new goroutines/channels/timers.
* Stay refactor-only (move/extract/rename) — behavior byte-for-byte unchanged.

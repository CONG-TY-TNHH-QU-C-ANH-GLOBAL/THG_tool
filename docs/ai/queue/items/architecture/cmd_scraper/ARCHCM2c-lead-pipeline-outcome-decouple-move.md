---
id: ARCHCM2c
status: REVIEW
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM2a, ARCHCM2b]
parallel_safe: false
branch: "chore/archcm2c-move-leadoutreach"
pr_url: ""
blocked_on: ""
boundary_target: transport-to-usecase
---

# ARCHCM2c — De-couple + move outbound_lead_outcome.go / outbound_lead_pipeline.go

## MOVE DONE (2026-06-30) — execution spine relocated to internal/services/facebook/leadoutreach
The finisher, on branch `chore/archcm2c-move-leadoutreach` (move-only, behavior-preserving;
senior-architect feasibility + senior-backend implementation + code-review verified).
- **New package `internal/services/facebook/leadoutreach`** (store-free): `Context`+`Config`+`New`,
  `ProcessLead`/`FormatResult`/`Mode` (exported), the 3 ports + `QueueOutcome` DTO, `State`
  (`Queued`/`Scanned` exported), and the pipeline/outcome/state/copilot-wording helpers — split
  across `context.go`/`pipeline.go`/`outcome.go`/`state.go`/`ports.go`/`copilot_wording.go`/`doc.go`,
  each ≤200 lines. The 4 cluster tests moved as internal `package leadoutreach` tests.
- **cmd stays the composition root:** `buildLeadOutreachContext` (resolves store→neutral, builds
  `leadoutreach.Config`, calls `New`) and the four `store*` adapters remain in cmd. Deleted the now-
  empty `outbound_lead_outcome.go` + `lifecycle_copilot.go`; the post path calls `leadoutreach.Mode`.
- Verbatim move: Vietnamese strings + `[queueLeadOutreach]` log line byte-identical; queue/dedup/
  policy semantics unchanged (recorder maps the store result 1:1). No `internal/store` import in the
  new package (import-boundary guard clean); cycle-free (cmd→leadoutreach one-way).

**ARCHCM2c COMPLETE pending merge** — all 4 seams + the move shipped. On merge → DONE.

## SEAM 4 DONE (2026-06-30) — commenting usecase decoupled from *store.Store; context is now store-FREE
Fourth brick, on branch `chore/archcm2c-commenting-store-decouple`. Removes the LAST and
critical-path store coupling — `commenting.Apply`'s `Input.DB *store.Store` (blast radius:
1 caller each for `commenting.Apply` and `ai.LoadOrgCommentPolicies`):
- **`internal/services/facebook/commenting`** no longer imports `internal/store`. `Input.DB`
  → `Input.Policies ai.CommentPolicies` (resolved in the cmd composition root) +
  `Input.PromptLog PromptLogSink` (new 1-method interface). `ai.LoadOrgCommentPolicies` moved
  to the caller; the prompt-log write goes through the injected sink. Extracted a testable
  `logDecision` helper.
- **cmd:** new `storePromptLog` adapter (pass-through over `Prompts().InsertSystemPromptLog`);
  `buildLeadOutreachContext` resolves `contacts`/`promptLog`/`commentPolicies` (policies under
  the exact original condition: `comment && reasoning != off`).
- **`leadOutreachContext` lost its `db *store.Store` field entirely** — it became unread once
  the commenting + contacts coupling moved to resolved fields, so it was deleted. The per-lead
  execution path (lead_pipeline + lead_outcome) is now **fully store-free**; only
  `buildLeadOutreachContext` (the composition-root adapter, by design) takes `*store.Store`.
- Store-free characterization tests (`TestLogDecision`/`_KnowledgeGap`) pin the prompt-log seam
  (mode-tagged action, `success == !KnowledgeGap`, marshaled payload). Existing `TestMode` kept.

**ARCHCM2c decoupling is essentially COMPLETE.** Both target files' execution paths hold no
`*store.Store`; the build layer is the intended cmd adapter. What remains is the MOVE itself
(relocate the execution spine + the consumer-owned ports/Input to the FB usecase package, leave
the build adapter in cmd) — a separate move-only slice, now unblocked.

Behavior-preserving notes: org comment policies are run-invariant, so resolving once per run ==
the original per-lead load (same value, fewer reads); `logDecision` keeps the best-effort
swallow (marshal-fail + ignored write error) verbatim. No RED zone touched.

## SEAM 3 DONE (2026-06-30) — lifecycle read port; outbound_lead_outcome.go is now STORE-FREE
Third decoupling brick, on branch `chore/archcm2c-lifecycle-read-port`. Removes the LAST
direct `*store.Store` read in the outcome path:
- **`outbound_lead_reader.go`:** added `leadLifecycleReader` interface (1 read-only method,
  neutral `models.LifecycleSummary` return) + `storeLeadLifecycle` pass-through adapter.
- `noEligibleCommentMessage` (lifecycle_copilot.go) signature changed `db *store.Store` →
  `lifecycle leadLifecycleReader` (1 caller — formatCommentResult); body reads via the port;
  **the file drops its `internal/store` import** (noEligible was its only store user).
- `leadOutreachContext` gains a `lifecycle` field, wired in `buildLeadOutreachContext`;
  `formatCommentResult` calls `noEligibleCommentMessage(ctx, c.lifecycle, …)`.
- Store-free characterization tests (fakeLifecycle) pin the enriched message + the
  error→bare-base fallback.

**Effect: `outbound_lead_outcome.go` no longer references `c.db` at all — it is fully
store-free and independently movable.** The remaining ARCHCM2c blocker is entirely in
`outbound_lead_pipeline.go`:
1. **`commenting.Apply` store-coupling (the only blocker left).** `commenting.Input.DB
   *store.Store` is used inside `internal/services/facebook/commenting` for
   `ai.LoadOrgCommentPolicies(in.DB, orgID)` (cascades into `internal/ai`) +
   `in.DB.Prompts().InsertSystemPromptLog(...)` (a write). Decoupling it is a CROSS-COMPONENT
   YELLOW seam owned by the commenting package (its public API takes the store) + an
   internal/ai cascade — NOT a bounded lead-pipeline slice. Needs its own feasibility pass /
   likely a commenting-API decision (does commenting own its data access or receive resolved
   policies + a prompt-log sink?). Deferred.

The build layer (`buildLeadOutreachContext`) stays in cmd as the composition-root adapter
(resolves store → neutral objects). Seams 1+2+3 are the reversible bricks; the move stays
BLOCKED only on the commenting decouple.


## SEAM 2 DONE (2026-06-30) — coverage read port
Second decoupling brick, on branch `chore/archcm2c-coverage-read-port`. Removes the
per-lead execution path's direct `*store.Store` coverage read:
- **New `cmd/scraper/outbound_lead_reader.go`:** `leadCoverageReader` interface (1
  read-only method, neutral `*models.LeadCoverageState` return — not a store re-export)
  + `storeLeadCoverage`, a pass-through cmd adapter over `*store.Store`.
- `leadOutreachContext` gains a `coverage leadCoverageReader` field (wired once in
  `buildLeadOutreachContext`); `coverageGate` now calls `c.coverage.GetLeadCoverageState`
  instead of `c.db.Leads().GetLeadCoverageState`.
- Store-free characterization tests (fakeCoverage) pin coverageGate's seam behavior:
  non-comment short-circuit (reader untouched), error-tolerant proceed, exact
  org/lead/website args passed. `models.EvaluateCoverage`/`DeriveActorPersona` semantics
  unchanged (already covered by multi-actor coverage tests).

**After Seam 2, the movable per-lead execution path's only remaining store coupling is
the cross-component `commenting.Apply` call** (`commenting.Input.DB *store.Store` +
`Contacts fbContactDirectory`). That is the critical-path blocker — a separate, larger
seam OWNED BY `internal/services/facebook/commenting` (its public API takes the store),
not the lead pipeline. The build layer (`buildLeadOutreachContext`) stays in cmd as the
composition-root adapter (it resolves store into neutral objects). Remaining seams:
1. **`commenting.Apply` store-coupling** — decouple `commenting.Input.DB` to an interface
   (commenting-package-owned). Critical-path blocker for the move.
2. **`noEligibleCommentMessage(ctx, c.db, …)`** (cmd-local free helper reading
   `Leads().LeadLifecycleSummary`) — needs a small read port or relocation.

Stays BLOCKED on those; Seams 1+2 are the reversible first bricks.


## SEAM 1 DONE (2026-06-29) — outbound persistence port (the RED-core decouple)
First behavior-preserving decoupling brick, on branch `chore/archcm2c-outbound-recorder-seam`.
The scariest concrete-store dependency in the movable per-lead path — the queue WRITE +
Knowledge outcome recording in `queueOutreachMessage` — is now behind a narrow consumer-owned
port instead of `*store.Store`:
- **New `cmd/scraper/outbound_lead_recorder.go`:** `outboundRecorder` interface (2 methods:
  `QueueOutbound` + `RecordOutcome`) + a neutral `queueOutcome` DTO (models/primitives only, so
  the port does NOT leak the store-tree `outbound.QueueResult`) + `storeOutboundRecorder`, a
  pass-through cmd adapter over `*store.Store` (uses the non-deprecated `Outbound().Queue`).
- `leadOutreachContext` gains an `outbound outboundRecorder` field (wired once in
  `buildLeadOutreachContext`); `queueOutreachMessage` no longer names `*store.Store`.
- The pure store-free `leadOutreachState` accumulator moved from lead_pipeline.go (was at the
  200-line cap) to lead_outcome.go beside its formatters — "extract pure logic first".
- Store-free characterization tests (fakeRecorder) pin the allowed / risk-block / no-retrievalID
  paths + the 24h cooldown + immutable CreatedBy. queue/dedup/policy semantics UNCHANGED (the
  concrete store still executes the write).

**Not a god interface:** one 2-method port, not a re-export of the store surface. **Partial
unblock** — the move is still blocked; remaining store coupling in the cluster (next seams):
1. `commenting.Apply` takes `commenting.Input.DB *store.Store` (and `Contacts fbContactDirectory`)
   — the `internal/services/facebook/commenting` package is itself store-coupled; decoupling it
   is a separate, larger seam and is the critical-path blocker for the move.
2. `noEligibleCommentMessage(ctx, c.db, ...)` (cmd-local, in formatCommentResult) — needs a small
   read port or relocation.
3. The build layer (`buildLeadOutreachContext`): `businessContextForOrg(db,…)`,
   `ai.LoadProfileForOrg(db,…)`, `fbContactDirectory{db}`, `knowledgeRuntime.NewBuilder(db.Knowledge())`,
   coverage read `c.db.Leads().GetLeadCoverageState`. The build is the composition-root adapter; most
   of it can STAY in cmd and pass already-resolved neutral objects, but the coverage read inside the
   per-lead `coverageGate` still needs a narrow read port before the move.

Stays BLOCKED on those follow-on seams; SEAM 1 is the reversible first brick.


## Goal
Move the remaining L3 files — `outbound_lead_outcome.go` and the
`outbound_lead_pipeline.go` orchestration spine — out of the composition root, after
de-coupling them from the cmd-local helpers they reach into.

> **TARGET CORRECTION (2026-06-28, per ARCHCM2b, founder-directed):** NOT
> `internal/outbound` — that package is the vertical-neutral spine and forbids
> `services/facebook` + `ai` imports. Both files import `services/facebook` (lead_pipeline
> also `ai` + knowledge), so they are FB+AI content logic whose home is the **FB usecase
> side** (`internal/services/facebook/...`). `cmd/scraper` builds adapters and calls the
> usecase. Plan the move against the FB usecase package, not `internal/outbound`.

## Component / domain
outbound lead pipeline + outcome recording.

## Blockers (why this is not yet READY)
Unlike comment_reasoning, these two files are glued to cmd-local helpers defined
outside the cluster:
- lead_outcome: `formatCommentResult`, `formatOutreachResult`,
  `noEligibleCommentMessage`, `queueOutreachMessage`, `recordSkip`.
- lead_pipeline: `coverageGate`, `businessContextForOrg`, `queueOutreachMessage`,
  `recordSkip`, `formatOutreachResult`, `prepareOutreachContent`, `processOutreachLead`,
  `fbContactDirectory`, plus one stray `argString(args,"template")`.

Each must be lifted into the cluster, injected, or relocated to a shared package
before the move, without changing behavior. lead_pipeline also calls into the L2
resolution layer via the queue facade, so its final shape depends on the L2 home
(ARCHCM2a) and on the `internal/outbound` package established by ARCHCM2b.

## Dependencies
ARCHCM2a (L2 home decided), ARCHCM2b (package + facade established).

## Risk notes
YELLOW move touching the outbound queue *call sites* (`QueueOutboundForOrg`,
`RecordOutcome`) — preserve queue/dedup/policy semantics EXACTLY (queue writes are RED
if altered). Behavior-preserving; characterization before each helper decouple.

## Validation
go build/test ./... ; scripts/check_topology.sh ; ai_validate.sh.

## Done criteria
lead_outcome + lead_pipeline in `internal/outbound`; shared cmd helpers decoupled;
no import cycle; queue semantics identical; guards green.

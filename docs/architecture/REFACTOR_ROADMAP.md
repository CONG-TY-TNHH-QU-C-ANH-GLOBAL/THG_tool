# Refactor Roadmap

**Status:** OFFICIAL STANDARD (sequencing). **Companion of** `ARCHITECTURE_STANDARD.md`.
Staged path from the code-as-of-now (`CURRENT_CODE_AUDIT.md`) to the standard. Every
phase is **additive-first, behavior-preserving where possible, independently
revertible**. No big-bang. Product features (Phase H) come AFTER the boundaries and
the outbox exist, so they are built right the first time.

**Global rules for every phase**
- A refactor-only PR changes NO behavior (move/rename/split only).
- A behavior-changing PR ships tests + typed reason codes.
- State PR type in the completion report.
- Each phase runs: `go test ./...`, `go vet ./...`, file-size, topology,
  tenant-isolation, component-structure, and `check_import_boundaries.sh`.

---

## Phase A — Architecture docs + guards  ✅ STARTED

**A.1 — docs (DONE, merged `6bd9efb6`):** the 8 standard docs + warn-only
`scripts/check_import_boundaries.sh`.

**A.2 — guardrails scaffold (this PR, `refactor/architecture-phase-a-guardrails`):**
- `doc.go` package markers at the target roots (`internal/platform`, `internal/drivers`,
  `internal/drivers/copilot`, `internal/services`, `internal/services/facebook`,
  `internal/outbound`, `internal/events`, `internal/knowledge`, `internal/brand`,
  `internal/notifications`, `internal/ai`) — empty boundary markers, no runtime code moved;
- `docs/architecture/MODULE_OWNERSHIP.yml` (machine-readable owner/target/status/phase);
- `docs/architecture/CURRENT_PACKAGE_INVENTORY.md` (per-package status + target + phase);
- hardened import guard (12 rules, rule/warning counts, known-gap + next-phase annotation);
- warn-only CI hook in `ci.yml` (`bash scripts/check_import_boundaries.sh || true`).

- **Goal:** turn the standard into enforceable, VISIBLE guardrails without moving
  runtime logic. Establish the contract + scaffolds before any code move.
- **Files/modules:** `docs/architecture/*`, `scripts/check_import_boundaries.sh`,
  `.github/workflows/ci.yml`, `internal/*/doc.go` (scaffolds only).
- **Behavior-change risk:** none (docs + empty packages + warn-only tooling/CI).
- **Rollback:** delete the docs/script/scaffolds; the CI step is `|| true`.
- **Tests/guards:** `check_import_boundaries.sh` exits 0 (12 rules, 4 known-gap
  warnings, 0 other); `go build ./...`/`go vet ./...` clean with the empty packages;
  all existing guards unchanged.

### ▶ Next PR recommendation

**Phase B — Pure AI boundary (move-only, lowest risk).** Separate the pure
intelligence (`internal/ai/comment` + generators) from the Copilot driver
(`agent*.go`/`intent_*.go`/`brain*.go`) so the `ai` package becomes import-clean.
This is move-only/behavior-preserving, directly retires the 4 `COPILOT_NO_DIRECT_REPO`
warnings' first half, and unblocks Phase D (typed `CommandBus`).
Alternative if FB sequencing is the priority: **Phase C — Facebook service boundary
inventory/move-only** (give FB workflows a `services/facebook` home). Do NOT schedule
product features (P1/P2 re-implementation, Phase H) until the boundaries + outbox
(Phase E) are in place.

## Architecture Foundation Sprint log (`refactor/architecture-foundation-sprint`)

One sprint, multiple independently-revertible commits. SAFE moves + additive scaffolds
only; risky moves deferred with evidence.

| Commit | Phase | Result |
|---|---|---|
| A | B (pure AI) | **DONE** — moved `BuildPersonaRule` (was `buildPersonaRule`) into `internal/ai/comment`; `go list -deps` proves comment purity (comment + models only). |
| — | B.2 (copilot driver) | ⚠️ **The earlier "import cycle" here was a FALSE POSITIVE — corrected.** The prior analysis claimed `classifier.go` consumed the driver's `buildDynamicSystemPrompt`; in fact `classifier.go` defines its OWN same-named **method** `func (c *Classifier) buildDynamicSystemPrompt()` — a symbol-name collision, not a cross-reference. A corrected scan (filtering method definitions + comments) found **zero** real staying→driver references. The driver move is cycle-free. **DONE** in `refactor/copilot-driver-move-b2` — see the B.2 row below. |
| B | D (ports) | **DONE (scaffold)** — `internal/outbound/ports.ActionExecutor` + `internal/services/facebook/ports.OutboundPlanner` (consumer-owned, compile-safe, NOT wired). Zero thg deps. |
| C | E (events) | **DONE (scaffold)** — `internal/events/{outbox,relay,bus}`; `outbox` has `Envelope`/`EventType`(×7)/`Status` TYPES only. No table, no relay, no migration. |
| — | C (FB runtime) | **DEFERRED — wide ripple / wrong-direction.** `fburl` (pure) has 8 importers incl. `internal/ai` → moving it under `services/facebook` would create an illegal `ai`→`services` edge (it's a cross-cutting platform-trust leaf, keep it out of the service). `leadingest` ripples to server+worker and is itself the Phase-E callback. Audit map below; runtime move deferred to a dedicated Phase C PR. |
| D | F (docs/guards) | **DONE** — this docs update + MODULE_OWNERSHIP.yml statuses. Import guard unchanged (paths didn't move out of `internal/ai`). |

### B.2 — Copilot driver move (`refactor/copilot-driver-move-b2`, after the false-positive correction)

**DONE, move-only, behavior-preserving.** Moved the Copilot driver + intent + routing
out of `internal/ai` into `internal/drivers/copilot` (15 production files + 6 tests):
`agent*.go` (Agent, brain, memory, preflight, prompt, request, responses, tools,
action_router), `intent_*.go` (×5), `routing_decision.go`. The cycle was debunked, so
the move is clean: `copilot → ai` is one-way (driver references only `ai.BusinessProfile`
+ `ai.ProfileFromContext`, qualified mechanically), and **no** staying-ai file references
a driver symbol, so `ai` does NOT import `copilot`.

- **Stayed in `internal/ai`** (NOT forced into copilot): `business.go`, `classifier.go`,
  `policy_gate.go`, `universal.go`, `comment_decision.go`, `group_scorer.go`, `msggen.go`,
  `pricer.go`, `profile_inference.go`, `selector*.go`. `business`/`classifier` hold
  `MessageGenerator` methods + `BusinessProfile`; `policy_gate` is comment/outbound policy
  (used by `outbound_actions.go`), not the prompt-routing driver.
- **External call sites updated** (wiring/adapter layers only): `internal/server/{agent/
  routes,server,skills/handlers}.go`, `internal/server/router.go`, `cmd/scraper/main.go`
  — `ai.Agent`→`copilot.Agent`, `ai.NewAgent`/`ai.NewBrainClient`→`copilot.*`,
  `ai.PromptIsSelfSufficient`→`copilot.*`. `ai.ClassifyIntent` stayed (`universal.go`).
- **Guard taxonomy refined honestly:** `COPILOT_NO_DIRECT_REPO` now points at
  `internal/drivers/copilot/agent.go` (the moved driver still imports store → Phase G).
  The store-coupled files that REMAINED in `ai` are tracked under a new, accurate
  `AI_STORE_COUPLED` rule (business/classifier/policy_gate → Phase G+). No warnings hidden.
- **promptbuilder extraction:** still useful (a neutral, testable prompt builder) but
  **deferred** — it was only needed to break the (non-existent) cycle, so it is no longer
  a blocker. Optional follow-up cleanup.

### Phase C migration audit map (what eventually moves to `services/facebook`)

| Source (today) | Eventually | Blocker / risk |
|---|---|---|
| `cmd/scraper/outbound_actions.go` `queueLeadOutreach` | FB part → `services/facebook`; neutral queue → `outbound` | god file (886 LOC), hot path — needs char-tests + the outbound neutral/FB split (Phase C/I) |
| `internal/jobhandlers/facebook_crawl` | `services/facebook` (crawl handler) | imported by worker + website ingestor; move ripples to `cmd/worker` |
| `internal/leadingest` | `services/facebook` (ingest) + emits `FacebookLeadCreated` | server + worker importers; OnLeadCreated is the Phase-E event target — move WITH the outbox |
| `internal/fburl` | stays a pure platform-trust leaf (NOT under services) | 8 importers incl. `internal/ai`; moving under services breaks the ai-no-services rule |
| connector / lead / comment / posting / inbox handlers | `services/facebook` + `connectors` (Phase F) | spread across `internal/server/agent` (23 files) + store domains |

## Phase B — Pure AI boundary

- **Status:** partially done in the foundation sprint (Commit A moved `BuildPersonaRule`).
  Remaining pure-comment extraction (`comment_decision.go` pure functions) is blocked by
  `MessageGenerator` methods + `BusinessProfile` coupling — see Phase B.2 / G.
- **Goal:** make the `ai` intelligence module import-clean and physically distinct from
  the Copilot driver. Catalog `internal/ai/comment` + pure generators as `ai`; mark
  `agent*.go`/`intent_*.go`/`brain*.go` as `drivers/copilot`.
- **Files/modules:** `internal/ai/*` (classification by header/comment first, optional
  later package move).
- **Behavior-change risk:** none if move-only; verify generators still import only
  `models`.
- **Rollback:** revert moves (mechanical).
- **Tests/guards:** boundary rule `AI_PURE` flips from warn to a documented exception
  list; `go test ./internal/ai/...`.

## Phase C — Facebook service boundary

- **Goal:** define `services/facebook` as the home of FB workflows; draw the line
  between vertical-neutral outbound and FB-specific target-URL/selector logic. Split
  `cmd/scraper/outbound_actions.go` neutral core ⟂ FB resolution.
- **Files/modules:** `cmd/scraper/*` orchestration, `internal/jobhandlers/facebook_crawl`,
  `internal/leadingest`, `internal/fburl` (stays pure).
- **Behavior-change risk:** medium (touches `queueLeadOutreach` hot path) — do as
  move-only with characterization tests pinned first.
- **Rollback:** revert the split commit; behavior identical by construction.
- **Tests/guards:** existing outbound + direct-comment tests stay green; boundary rule
  `OUTBOUND_NO_FACEBOOK` enforced.

## Phase D — Ports / handler registry

- **Goal:** replace the untyped `ActionHandler(map[string]any)` with a typed
  consumer-owned `CommandBus` (driver) + `OutboundPlanner`/`ActionExecutor` ports
  (`PORTS_AND_ADAPTERS.md`). Wire at composition root only.
- **Files/modules:** `drivers/copilot`, `services/facebook`, `internal/store/outbound`,
  `cmd/scraper/main.go`.
- **Behavior-change risk:** medium — same routing, new typed seam. Tests pin routing.
- **Rollback:** keep the legacy `ActionHandler` path behind the new port for one cycle;
  revert is removing the new port.
- **Tests/guards:** routing characterization tests; no `map[string]any` cross-module
  contracts for new code.

### D.0 — Sonar cognitive-complexity hygiene (pre-Phase-D, refactor-only)  ✅ DONE

A pure-readability batch that does **not** start the Phase D typed-`CommandBus`
redesign — it only de-nests the legacy dispatcher so the eventual extraction is
easier and the `go:S3776` debt stops blocking the quality gate.

- **Branch:** `refactor/sonar-cognitive-agent-actions` (from `origin/main` @ `2afbd2a7`).
- **Sonar issue/rule fixed:** `AZ7askWm1xM_XIKj2DHI` — `go:S3776` (cognitive
  complexity 55 → ≤15) on `cmd/scraper/agent_actions.go:14` `makeAgentActionHandler`.
- **Changed files:** `cmd/scraper/agent_actions.go` (this `REFACTOR_ROADMAP.md` note).
- **What changed:** introduced an unexported `agentActionRouter` struct holding the
  existing deps (`db`, `jobStore`, `msgGen`, `notify`, `intake`); `makeAgentActionHandler`
  keeps its exact signature and returns `r.handle`; `handle` is now a thin dispatch
  switch and each former case body moved verbatim into a small private method.
- **Refactor-only or behavior-changing:** **refactor-only.**
- **Behavior preserved:** yes — identical action set, args, `context.Background()`
  usage, error strings, comments, and per-branch call order; the outbound safety
  spine (`runPooledOutreach`/`commentSinglePost`/`guardFacebookWriteAccount`/
  `queueGroupPost`/`queueProfilePost`) and `org_id` tenant scoping are untouched.
  Does NOT introduce a typed CommandBus or cross any package boundary (stays in
  `cmd/scraper`); Phase D itself remains open.
- **Validation:** `gofmt` clean; `go vet ./...` clean; `go build ./...` clean;
  `go test ./...` PASS (full suite; `-race` not runnable in this Windows env —
  `CGO_ENABLED=0`, no C compiler — run it in CI/Linux); `cmd/scraper` coverage 34.5%;
  `check_import_boundaries.sh` exit 0 (4 pre-existing known-gap warnings, **0 new**);
  `check_file_size.py` PASS.
- **Remaining risks:** none identified (mechanical move; same package, same calls).
- **Remaining Sonar `go:S3776` issues (NOT in this batch):** still open in
  `cmd/scraper/{main.go:28(64), crawl_runtime.go:19(22)/114(29), outbound_actions.go:90(95)/806(33),
  skills_register.go:331(22)}`, `internal/drivers/copilot/{agent.go:225(97), agent_brain.go:150(39),
  intent_router.go:14(28)}`, `internal/leadingest/ingest.go:223(105)`, and ~35 more across
  server/store/ai — to be taken one batch at a time after review.

## Phase E — Transactional outbox foundation  ★ keystone

- **Goal:** introduce `outbox_events` table + relay + consumed-events idempotency,
  ADDITIVELY alongside the in-memory bus. Migrate the FIRST critical event
  (`FacebookPostImported`).
- **Files/modules:** `internal/events` (or new `internal/outbox`), a migration for
  `outbox_events`, `cmd/scraper`/`cmd/worker` relay wiring.
- **Behavior-change risk:** medium — new infra; keep old callbacks until each event is
  migrated. The migration itself is additive (CREATE TABLE/INDEX idempotent).
- **Rollback:** stop the relay; drop the table (no-op safe); callbacks still work.
- **Tests/guards:** outbox write-in-tx test, relay retry/idempotency test, poison→dead
  test. New `EVENTS_NO_SERVICE_IMPORT` boundary rule.

## Phase F — Connector pull / outbox hardening

- **Goal:** codify the connector state machine + action lifecycle
  (`CONNECTOR_STATE_MACHINE.md`) into one readiness module; ensure CAS/lease + lease-
  expiry safety net are uniform; emit `ConnectorReadyChanged`/`ConnectorChallengeRequired`
  via the outbox.
- **Files/modules:** `internal/store/connectors`, `internal/store/outbound`
  (claim/lease), `internal/server/agent`.
- **Behavior-change risk:** medium (claim path) — pin with double-claim + lease-expiry
  tests.
- **Rollback:** per-commit revert; CAS semantics unchanged.
- **Tests/guards:** no-double-claim test across two connectors; `human_required` on
  challenge.

## Phase G — Copilot driver cleanup

- **Goal:** remove the driver's direct `*store.Store` dependency; it depends only on the
  `CommandBus` port. Driver becomes a thin NL→command translator.
- **Files/modules:** `internal/ai/agent*.go` → `drivers/copilot`.
- **Behavior-change risk:** low-medium; routing tests pin behavior.
- **Rollback:** revert; legacy path intact.
- **Tests/guards:** boundary rule `COPILOT_NO_DIRECT_REPO` flips warn→clean.

## Phase H — Product features (re-implemented on the standard)

The features prototyped in the paused stack are RE-IMPLEMENTED here, correctly:

- **H1 — Direct-post intake → comment continuation (durable).** Re-implemented as a
  focused **process manager**, not the generic outbox (which would be overkill): a
  `direct_post_comment_workflows` table + a DB-polling poller that observes the post
  lead and queues the comment, idempotent via CAS + a two-key model (`intake_key` vs
  `idempotency_key`). NO `user_context` KV, NO in-memory callback as source of truth.
  Spec: `specs/DIRECT_POST_INTAKE_WORKFLOW.md`. **PR-1 (data foundation) DONE** —
  migration `0022` + coordination store (CRUD + CAS/lease) + `GetPostLeadByRef`.
  **PR-2 (runtime) DONE** — the `directPostIntake` service (unknown post → import +
  async ack, replacing scan-required), the `runDirectPostIntakeScheduler` DB poller
  (observe lead → queue comment, CAS-guarded, bounded retry, graceful shutdown), wired
  in `cmd/scraper/main.go`. Telegram stays the existing lead-created notification; a
  future outbox (Phase E) hardens exactly-once delivery.
- **H2 — Typo/multilingual NLU.** Port P2's guarded fuzzy verbs (`commend`/`cmt`,
  scope-gated) into the `drivers/copilot` intent layer. P2 is behavior-isolated and can
  largely cherry-pick once the driver boundary (Phase G) lands.
- **Behavior-change risk:** feature-level, fully tested; but now durable + service-
  scoped.
- **Rollback:** feature flag / revert the process manager registration.
- **Tests/guards:** P1/P2 test suites carried forward + outbox idempotency tests.

## Phase I — Outbound PR2B cleanup

- **Goal:** the V2 Outbound breaking cleanup — drop legacy `status`/`claimed_by`/… 
  columns, remove `LegacyStatusFor`, retire the 28 deprecated bridge wrappers, finish
  file split.
- **Files/modules:** `internal/store/outbound*`, `internal/models/outbound_state.go`,
  callers of the deprecated aliases.
- **Behavior-change risk:** HIGH (breaking) — gated on `specs/V2_OUTBOUND_REFACTOR_
  DESIGN.md` prereqs (PR1 deployed, ≥1 week production traffic, reconciler verifies
  ledger==state). Must NOT stack on feature work (it sits on the same hot path).
- **Rollback:** coordinated; columns dropped only after read-paths verified gone.
- **Tests/guards:** full outbound suite; contract checks for extension/webhook.

---

## Dependency order (why this sequence)

```
A (contract) ─▶ B (pure AI) ─▶ C (FB boundary) ─▶ D (ports)
                                                     │
                                                     ▼
                                            E (outbox) ★ keystone
                                              │       │
                                              ▼       ▼
                                  F (connector)   G (copilot cleanup)
                                              │       │
                                              └───┬───┘
                                                  ▼
                                          H (features: H1 needs E)
                                                  │
                                                  ▼
                                          I (outbound PR2B, gated, last)
```

- Features (H) depend on the outbox (E) and the FB boundary (C). That is precisely why
  the paused P1/P2 stack is held: P1 is a useful prototype but belongs on E.
- I (PR2B) is last and gated — it touches the same spine every feature uses; doing it
  before the features are stable risks mixing failure domains.

## Paused feature-stack disposition (2026-06-14)

The accelerated direct-comment sprint produced a stacked prototype. Disposition:

- **P0** `fix/copilot-direct-comment-routing` (`4d2b8335`, incl. fburl `331dc602`):
  **MAY merge separately as a production hotfix after review.** It is a real,
  isolated routing fix (direct-comment early-bypass + user_id/user_role threading);
  it does not depend on any later phase.
- **P1** `feat/direct-comment-import-unknown-post` (`4b651dcb`): **prototype / reference
  ONLY — do NOT merge as-is.** Re-implement on the outbox (Phase **H1**): a durable
  `FacebookPostImported` event + a `services/facebook` process manager with its own
  continuation table, not `user_context`.
- **P2** `feat/copilot-typo-multilingual-intent` (`167a2e72`): **do NOT merge while
  stacked on P1.** Preserve tests/ideas; rebase / cherry-pick into `drivers/copilot`
  after Phase **G** (Phase **H2**).
- **Outbound PR2B**: **remains DEFERRED** (Phase **I**, gated on the
  `specs/V2_OUTBOUND_REFACTOR_DESIGN.md` prerequisites).

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
| — | B.2 (copilot driver) | **DEFERRED — import cycle.** `buildDynamicSystemPrompt` (defined in driver `agent_prompt.go`) is consumed by `classifier.go` (a staying generator, also store-coupled). Moving the driver → `ai`→`copilot` (via classifier) while `copilot`→`ai` (driver uses `MessageGenerator`/`BusinessProfile`) = cycle. Export-surface the other way is 0, so the ONLY blocker is this back-reference + `classifier`/`buildDynamicSystemPrompt`/`MessageGenerator` entanglement. Needs a decouple (behavior-sensitive), not a move-only. The 4 `COPILOT_NO_DIRECT_REPO` warnings correctly stay on `internal/ai/...`. |
| B | D (ports) | **DONE (scaffold)** — `internal/outbound/ports.ActionExecutor` + `internal/services/facebook/ports.OutboundPlanner` (consumer-owned, compile-safe, NOT wired). Zero thg deps. |
| C | E (events) | **DONE (scaffold)** — `internal/events/{outbox,relay,bus}`; `outbox` has `Envelope`/`EventType`(×7)/`Status` TYPES only. No table, no relay, no migration. |
| — | C (FB runtime) | **DEFERRED — wide ripple / wrong-direction.** `fburl` (pure) has 8 importers incl. `internal/ai` → moving it under `services/facebook` would create an illegal `ai`→`services` edge (it's a cross-cutting platform-trust leaf, keep it out of the service). `leadingest` ripples to server+worker and is itself the Phase-E callback. Audit map below; runtime move deferred to a dedicated Phase C PR. |
| D | F (docs/guards) | **DONE** — this docs update + MODULE_OWNERSHIP.yml statuses. Import guard unchanged (paths didn't move out of `internal/ai`). |

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

- **H1 — Direct-comment unknown-post import continuation.** Re-do P1 on the outbox: a
  `FacebookPostImported` event + a process manager in `services/facebook` with its own
  continuation table (not `user_context`), idempotent by event id, working on BOTH
  ingestion paths (connector + worker). Reuse the proven id-tolerant matching and the
  `queueLeadOutreach` gates.
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

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

## Phase A — Architecture docs + guards  ◀ (this PR)

- **Goal:** publish the standard (`docs/architecture/*`) + warn-only import-boundary
  script. Establish the contract before moving code.
- **Files/modules:** `docs/architecture/*`, `scripts/check_import_boundaries.sh`.
- **Behavior-change risk:** none (docs + warn-only tooling).
- **Rollback:** delete the docs/script; nothing references them at runtime.
- **Tests/guards:** `check_import_boundaries.sh` exits 0; existing guards unchanged.

## Phase B — Pure AI boundary

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

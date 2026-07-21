---
doc_type: architecture
status: active
owner: platform
last_reviewed: 2026-06-28
related_pr_or_issue: chore/docs2-architecture-backlinks-frontmatter
---

# Current Code Audit (2026-06-14)

> Part of the [architecture docs index](INDEX.md).

**Status:** AUDIT ONLY — no code is moved or changed by this document.
Honest assessment of how the code-as-of-`d8871712` compares to the standard in
`ARCHITECTURE_STANDARD.md`. Severity: 🟢 already aligned · 🟡 partial / tolerated ·
🔴 gap to close (with roadmap phase).

Method: static import scan + the existing guards (`check_topology.sh`,
`check_tenant_isolation.sh`, `check_component_structure.py`) + targeted greps.

---

## 1. AI as pure intelligence

- 🟢 `internal/ai/comment` imports **only** `internal/models`. Pure, platform-neutral,
  matches the binding rule exactly.
- 🟡 `internal/ai` top-level **mixes two logical modules**: the pure generators AND the
  Copilot *driver*. `internal/ai/agent.go`, `business.go`, `classifier.go`,
  `policy_gate.go` import `internal/store`. That is correct for a *driver* but means
  the `ai` package as a whole is not import-clean. **Action:** treat `internal/ai/
  agent*.go`+`intent_*.go`+`brain*.go` as `drivers/copilot` (separate logical module),
  keep the pure generators import-clean. Roadmap Phase B/G.
- 🟢 `internal/ai` imports **no** outbound/connector/browsergateway — intelligence has
  no execution coupling.

## 2. Copilot intent / router

- 🟢 Already split into a clean pipeline: `intent_types.go`, `intent_lexicon.go`,
  `intent_normalize.go`, `intent_entities.go`, `intent_router.go` — import only
  `fburl` + stdlib (per `specs/domains/facebook-sales-intelligence/features/sales-copilot/technical.md`). Good
  consumer-side shape.
- 🟡 The driver still dispatches through `Agent.ActionHandler func(string,
  map[string]any)` injected from `cmd/scraper`, and `Agent` holds `*store.Store`. Works,
  but the cross-module contract is an untyped bag and the driver imports the store.
  **Action:** typed `CommandBus` port (Ports doc §3). Roadmap Phase D/G.

## 3. Facebook-specific code (spread, not a module)

- 🟡 Facebook logic is spread across `internal/fburl` (URL trust — clean, pure),
  `internal/jobhandlers/facebook_crawl`, `internal/leadingest`, and `cmd/scraper/*`
  orchestration (`queueLeadOutreach`, `commentSinglePost`, crawl submission). There is
  no single `services/facebook` boundary yet. **Action:** define the boundary
  (Phase C) before adding more FB workflow surface.
- 🟢 `internal/fburl` is the single host-anchored FB URL trust source; intelligence and
  routing delegate to it. Keep.

## 4. internal/ai/comment & root ai comment files

- 🟢 `internal/ai/comment` extracted, pure (models-only).
- 🟡 Some comment-adjacent decision code still lives in root `internal/ai`
  (`comment_decision.go`) alongside the driver — acceptable while pure, but should be
  catalogued under the intelligence module, not the driver, to keep the boundary clear.

## 5. Connector / crawl / lead / comment / inbox / posting / leaderboard

- 🟢 Connector readiness centralized in `connectors.PickReadyConnector` (picker ==
  dispatcher). Pull endpoints in `internal/server/agent`.
- 🟡 Two ingestion paths wire `OnLeadCreated` independently — `cmd/worker/main.go`
  (SQLite queue) and `internal/server/agent/crawl.go` (extension result). Cross-module
  reactions are **composition-root callbacks**, not durable events. This is the core
  thing the transactional outbox replaces (Phase E). The P1 prototype's
  `SetPostLeadImportHook` is an instance of this pattern.
- 🟢 Leaderboard/attribution derived from the append-only ledger
  (`coordination/attribution.go`) — projection, not a stored CRM status. Good.

## 6. Outbound queue / ledger / policy / attempts

- 🟢 `internal/store/outbound` is import-clean (models + runtime/events + dbutil).
  Split into focused files (claim/dedup/lease/policy/queue/transition/finalize).
- 🟢 **No raw `execution_attempts` / `action_ledger` writes outside
  `internal/store/coordination`** — confirmed by grep; the append-only invariant holds.
- 🟡 `cmd/scraper/outbound_actions.go` (886 LOC, allowlisted legacy) is the FB-adjacent
  application orchestrator (`queueLeadOutreach`) — it mixes vertical-neutral queueing
  with FB target-URL resolution. **Action:** split neutral core from FB specifics
  (Phase C/I).
- 🟡 28 `Deprecated` L2 bridge wrappers in `internal/store/outbound_aliases.go`
  (tracked by `check_topology.sh` gate 9). Removal is V2 Outbound **PR2**, gated on
  production verification. Do not bundle with feature work.
- 🟡 Legacy `status` column + `models.LegacyStatusFor()` still present
  (`internal/models/outbound_state.go`). Drop is V2 PR2. Tolerated until then.

## 7. Telegram / event hooks / notifications

- 🟡 `internal/telegram/control` (notifications) imports `internal/store` and renders
  `LeadNotice`/`ActionNotice` — it is a sink, which is correct, but it is fed by direct
  calls (`tgEvents.NotifyLead`) inside crawl/outbox handlers rather than by subscribing
  to durable events. **Action:** make notifications a subscriber of outbox events
  (Phase E). Verify the `storetest` import seen in the control package is test-only.
- 🟢 Telegram is already routed through shared backend logic (not a separate command
  path) per `specs/TELEGRAM_*`. Keep.

## 8. Global store access / god files

- 🟡 `*store.Store` is a wide handle passed widely; many call sites reach multiple
  domains through one object. The domain accessors (`s.Outbound()`, `s.Leads()`) give
  structure, but nothing prevents a module from reaching a domain it shouldn't. The
  import-boundary script makes the worst cases visible (warn-only).
- 🟡 God-file watchlist (top offenders, all allowlisted legacy): `cmd/scraper/
  outbound_actions.go` (886), `internal/ai/msggen.go` (726), `internal/server/
  workspace/watchers.go` (655) & `handlers.go` (635), `internal/store/crawl/
  intents.go` (622), `internal/store/coordination/execution_attempts.go` (612). Each
  touch should extract, per the 200-line rule baseline.

## 9. Import-cycle risks

- 🟢 Go compiles clean (no actual cycles). The store domain dependency direction (L1 in
  `internal/store/DOMAINS.md`) is enforced by review + `check_topology.sh`.
- 🟡 The latent risk is the Copilot driver ↔ application wiring: if `internal/ai` (the
  driver) ever imported a service workflow that imports `internal/ai` (the
  intelligence), a cycle appears. The fix is the typed `CommandBus` port owned by the
  driver (Ports doc), which keeps the dependency one-way. The current
  `ActionHandler` callback already avoids the cycle the right way (injection at `main`)
  — formalize it as a typed port.

## 10. Raw execution_attempts writes outside coordination

- 🟢 **None.** Verified: `grep INSERT INTO (execution_attempts|action_ledger)` returns
  only `internal/store/coordination`. The single-writer rule for the append-only spine
  is intact and should be guarded going forward.

---

## Summary scorecard

| Area | State |
|---|---|
| AI intelligence purity (`ai/comment`) | 🟢 |
| Copilot driver / store coupling | 🟡 (typed port pending) |
| Facebook service boundary | 🟡 (spread; define before growth) |
| Outbound spine cleanliness | 🟢 core / 🟡 FB-adjacent orchestrator |
| Append-only single-writer | 🟢 |
| Durable events / outbox | 🔴 (in-memory callbacks today — Phase E) |
| Notifications as a sink | 🟡 (direct calls, not event-subscribed) |
| Tenant isolation | 🟢 (guard green) |
| God files | 🟡 (legacy baseline, extract-on-touch) |

**Biggest single gap:** no durable transactional outbox — critical cross-module
reactions ride in-memory composition-root callbacks (Phase E is the keystone that
unblocks re-implementing the P1 import-continuation feature correctly).

# Current Package Inventory

**Status:** AUDIT (Phase A) — describes `internal/` as-of-now (base commit
`6bd9efb6`). No code is moved by this document. Companion of
`CURRENT_CODE_AUDIT.md` (which is the gap/severity view) and `MODULE_OWNERSHIP.yml`
(machine-readable owner/target/status). Counts are production `.go` files at the
package root (excluding `_test.go` and subpackages).

Status legend: 🟢 aligned · 🟡 partial/tolerated · 🔴 gap. "Target module" /
"Phase" reference `ARCHITECTURE_STANDARD.md` + `REFACTOR_ROADMAP.md`.

---

## Intelligence & Copilot driver

| Package | Files | Responsibility (current) | Status | Target module | Phase |
|---|---|---|---|---|---|
| `internal/ai` | 28 | MIXED: pure generators (msggen, classifier schemas) **and** the Copilot driver (agent*.go, intent_*.go, brain*.go). Driver holds `*store.Store` + untyped `ActionHandler`. | 🟡 | split: `ai` (pure) + `drivers/copilot` (driver) | B + D/G |
| `internal/ai/comment` | 5 | Pure comment decision/repair intelligence. Imports **only** `models`. | 🟢 | `ai` (intelligence) | — (already clean) |
| `internal/fburl` | 2 | Host-anchored Facebook URL trust + canonicalization. Pure. | 🟢 | `services/facebook` (platform-trust leaf) | C (stays pure) |

**Obvious misplacements:** the copilot driver files (`agent.go`, `business.go`,
`classifier.go`, `policy_gate.go`, `intent_*.go`, `brain_*.go`) live under
`internal/ai` but are an inbound **driver**, not intelligence — they belong in
`internal/drivers/copilot` and must shed the direct store dependency (the
`COPILOT_NO_DIRECT_REPO` warnings). `internal/ai/comment_decision.go` (pure) is fine
to stay as intelligence but should be catalogued under the `ai` module, not the driver.

## Store domains (data layer)

| Package | Files | Responsibility | Status | Target module | Phase |
|---|---|---|---|---|---|
| `internal/store` | 18 | infra (Store, schema, migrations) + top-level domains (users/org/leads/identities legacy). | 🟡 | platform(users/org) + per-domain | B/C |
| `internal/store/coordination` | 18 | append-only `execution_attempts` + `action_ledger`, reverify, attribution, behaviour caps. **Sole writer** of the ledger. | 🟢 | `outbound` (coordination) | — (invariant holds) |
| `internal/store/outbound` | 11 | queue/dedup/claim/lease/transition/finalize/policy. Import-clean (models+events+dbutil). | 🟢 | `outbound` | C/I (split orchestrator) |
| `internal/store/connectors` | 13 | connector commands/pairing/screenshots/policy, agent tokens, selector cache. | 🟢 | `connectors` | F |
| `internal/store/crawl` | 6 | posts/groups/jobs/intents crawl artifacts. | 🟢 | `crawl/jobs` | C |
| `internal/store/leads` | 12 | leads, engagement projection, classification_log, `user_context` KV. | 🟡 | `leads` (KV usage flagged) | C/H |

**Note (append-only):** confirmed **zero** raw `execution_attempts`/`action_ledger`
writes outside `internal/store/coordination`. The single-writer rule is intact.

## Crawl / jobs / ingest / scoring

| Package | Files | Responsibility | Status | Target module | Phase |
|---|---|---|---|---|---|
| `internal/jobs` | 5 | crawl job Task/Job model, Store, scheduler (`Submit`/`Claim`/`Complete`). | 🟢 | `crawl/jobs` (infra) | C |
| `internal/jobhandlers` | 0 (subpkgs) | crawl handler(s); `facebook_crawl` subpackage runs the crawl + `IngestPost`. | 🟡 | `services/facebook` (FB handler) | C |
| `internal/leadingest` | 1 | `IngestPost` (real-content gate) + `OnLeadCreated` hook. Shared by both ingestion paths. | 🟡 | `services/facebook` (ingest) + emits events | C/E |
| `internal/scoring` | 1 | lead scoring config/scorer. | 🟢 | `ai`/`services` (via port) | B/C |

**Obvious misplacement:** `internal/leadingest` fires `OnLeadCreated` consumed by
composition-root callbacks (worker + server). That cross-module reaction should
become a durable `FacebookLeadCreated`/`FacebookPostImported` event (Phase E), not a
callback.

## Connector / browser

| Package | Files | Responsibility | Status | Target module | Phase |
|---|---|---|---|---|---|
| `internal/server/agent` | 23 | extension driver: heartbeat, crawl-result, outbox, commands, screenshots. | 🟡 | `drivers/connector` | F |
| `internal/browsergateway` | 1 | stream-status constants / browser gateway contract. | 🟢 | `connectors` | F |

**Note:** `internal/server/agent` is large (23 files) and mixes the connector driver
with some outbound/reverify handlers — a hotspot for the `drivers/connector` extraction.

## Outbound (application orchestrator)

| Package | Files | Responsibility | Status | Target module | Phase |
|---|---|---|---|---|---|
| `cmd/scraper/outbound_actions.go` | 1 (886 LOC) | `queueLeadOutreach` — the outbound orchestrator; mixes vertical-neutral queueing with FB target-URL resolution. Legacy god file (allowlisted). | 🟡 | `services/facebook` (FB part) + `outbound` (neutral part) | C/I |
| `internal/store/outbound_aliases.go` | 1 | 28 Deprecated L2 bridge wrappers (e.g. `QueueOutboundForOrg`). | 🟡 | remove | I (PR2B, gated) |

## Notifications / Telegram

| Package | Files | Responsibility | Status | Target module | Phase |
|---|---|---|---|---|---|
| `internal/telegram/control` | 10 | per-org Telegram channel notifications (`NotifyLead`/`NotifyAction`). A sink. | 🟡 | `notifications` | E |
| `internal/server/system/notifications.go` | 1 | in-app bell rows. | 🟡 | `notifications` | E |

**Note:** notifications are fed by **direct calls** (`tgEvents.NotifyLead`) inside
crawl/outbox handlers, not by subscribing to durable events — the Phase E change.

## Knowledge

| Package | Files | Responsibility | Status | Target module | Phase |
|---|---|---|---|---|---|
| `internal/store/knowledge` | (subpkg) | knowledge assets/sources/events/feedback. | 🟢 | `knowledge` | post-C |
| `internal/workspace_knowledge` | 1 (+subpkgs) | ingestion/retrieval/soak. | 🟡 | `knowledge` | post-C |

---

## Phase-A scaffold packages (this PR, empty markers)

`internal/platform`, `internal/drivers`, `internal/drivers/copilot`,
`internal/services`, `internal/services/facebook`, `internal/outbound`,
`internal/knowledge`, `internal/brand`, `internal/notifications` (new `doc.go` only);
`internal/events`, `internal/ai` (added `doc.go` to existing packages). They build
clean and contain NO runtime code — they mark target boundaries so future move-only
PRs have a destination and the import guard has paths to check.

## PR26C marker packages (target boundary markers, no runtime code)

These `doc.go`-only packages mark target boundaries from `DIAGRAM_RECONCILIATION.md`
§6. They build clean and contain **no runtime code**; the runtime still lives at the
listed current paths until a reviewed move-only PR migrates it.

| Marker package | Status | Current code still lives in |
|---|---|---|
| `internal/drivers/http` | marker only | `internal/server/*` (REST/SSE handlers) |
| `internal/drivers/telegram` | marker only | `internal/telegram`, `internal/server/telegram` |
| `internal/drivers/connector` | marker only | `internal/server/agent/*` (pull-based outbox; **controlled zone**) |
| `internal/connectors` | marker only | `internal/store/connectors`, `internal/browsergateway`, `internal/cdpclient`, `local-connector-extension/` |
| `internal/crawler` | marker only | `internal/jobs`, `internal/jobhandlers`, `internal/store/crawl`, `cmd/worker` |
| `internal/services/taobao` | future marker | `internal/platform/services/resolver` (stub only) |
| `internal/services/supplier1688` | future marker | `internal/platform/services/resolver` (stub `alibaba1688.go`) |
| `internal/automation` | marker only | none yet — cross-vertical glue; **must not become `common`/`utils`** |

**1688 naming (canonical):** the Go module path is **`internal/services/supplier1688`**
(a Go package name cannot start with a digit, so `internal/services/1688` is invalid);
the product/platform label remains **"1688"**. The existing resolver stub
`internal/platform/services/resolver/alibaba1688.go` is **not** renamed — `supplier1688`
is the future *service-module* path unless a later ADR changes it.

## Foundation sprint deltas (since Phase A)

- **Moved:** `internal/ai/comment_persona.go` → `internal/ai/comment/persona.go`
  (`buildPersonaRule` → `BuildPersonaRule`). `internal/ai/comment` is now 6 files,
  still pure (comment + models only, proven by `go list -deps`).
- **Added (additive scaffolds, not wired):** `internal/outbound/ports`
  (`ActionExecutor`), `internal/services/facebook/ports` (`OutboundPlanner`),
  `internal/events/{outbox,relay,bus}` (`outbox` has `Envelope`/`EventType`/`Status`
  types only — no table/relay/migration).
- **Deferred with documented blockers:** Phase B.2 copilot-driver move (import cycle
  via `buildDynamicSystemPrompt`↔`classifier.go`); Phase C FB-runtime moves (fburl
  cross-cutting + leadingest server/worker ripple). See REFACTOR_ROADMAP.md sprint log.
- Root `internal/ai` is now 27 production files (was 28); still holds the copilot
  driver — the 4 `COPILOT_NO_DIRECT_REPO` warnings correctly remain there.

## B.2 update — Copilot driver moved out of `internal/ai`

The Copilot driver/intent/routing (15 files + 6 tests: `agent*.go`, `intent_*.go`,
`routing_decision.go`) **moved** from `internal/ai` to **`internal/drivers/copilot`**
(move-only, behavior-preserving). The earlier "cycle" blocker was a FALSE POSITIVE
(`classifier.go`'s own same-named `buildDynamicSystemPrompt` method). `copilot → ai`
is one-way (`ai.BusinessProfile`/`ai.ProfileFromContext` only); `ai` does not import
`copilot`. Store-coupled files `business.go`/`classifier.go`/`policy_gate.go` **stayed**
in `ai` (generators + comment/outbound policy, not the driver) and are now tracked under
the new `AI_STORE_COUPLED` guard rule, distinct from `COPILOT_NO_DIRECT_REPO` (which now
correctly points at `internal/drivers/copilot/agent.go`). `internal/ai` shrank from 27 to
12 production files.

## P1 data foundation (H1, PR-1)

`internal/store/coordination` now owns `direct_post_comment_workflows` (migration
`0022`): the durable process-manager state for direct-post intake → comment
continuation (`direct_post_workflow.go` + `direct_post_workflow_transitions.go`, CRUD +
CAS/lease). `internal/store/leads` gained `GetPostLeadByRef` (post-only lookup,
excludes commenter leads). **PR-2 runtime** added in `cmd/scraper`:
`direct_post_intake.go` (the `directPostIntake` service — unknown post → import +
async ack) and `direct_post_intake_scheduler.go` (the `runDirectPostIntakeScheduler`
DB poller → observe lead → queue comment). `commentSinglePost` now uses
`GetPostLeadByRef` + the intake service instead of the scan-required copy. `jobs.Store`
gained `Close()`. Spec: `specs/DIRECT_POST_INTAKE_WORKFLOW.md`.

## Biggest inventory takeaways

1. **`internal/ai` is now intelligence/generation/scoring** — the Copilot driver moved
   to `internal/drivers/copilot` (B.2). Remaining `ai` store coupling
   (business/classifier/policy_gate) is the `AI_STORE_COUPLED` gap (Phase G+).
2. **Facebook logic is spread** across `cmd/scraper`, `jobhandlers/facebook_crawl`,
   `leadingest`, `fburl` — no `services/facebook` home yet (Phase C).
3. **Cross-module reactions are callbacks, not events** (`leadingest.OnLeadCreated`,
   `tgEvents.NotifyLead`) — the durable-outbox keystone (Phase E).
4. **Append-only + tenant isolation are already clean** — do not regress them.

---
doc_type: architecture
status: active
owner: platform
last_reviewed: 2026-06-27
related_pr_or_issue: docs/architecture-boundary-map
---

# Boundary Migration Playbook

**Status:** OFFICIAL STANDARD (process). **Companion of**
[`ARCHITECTURE_STANDARD.md`](ARCHITECTURE_STANDARD.md) (target shape),
[`MODULE_BOUNDARIES.md`](MODULE_BOUNDARIES.md) (the per-module import rules — the
authority; this doc does not restate them per-module), and
[`REFACTOR_ROADMAP.md`](REFACTOR_ROADMAP.md) (sequencing record).

Purpose: turn the architecture queue from a bag of "file split / refactor" items
into a **staged boundary migration**. It gives each item a *boundary target* (what
layer move it serves), a *feasibility gate* before any code moves, and a
GREEN/YELLOW/RED lane. A file split is only worth doing if it is prep for a clean
boundary; a boundary move is only allowed once the seam is clean.

## 1. Target layer map

Imports point **downward only**. (This is the migration lens over
`MODULE_BOUNDARIES.md`; that doc remains the per-module authority.)

| Layer | Packages | Responsibility |
|---|---|---|
| Composition root | `cmd/*` | Wiring/bootstrap only. Builds adapters, injects ports, starts the app. No business rules. |
| Transport / controller | `internal/server/*` (+ `internal/telegram/*`) | HTTP/Telegram in/out. Parse → call a usecase → render. No business decisions, no direct store fan-out beyond a thin read. |
| Application / usecase | `internal/services/*` (+ today `internal/drivers/copilot` orchestration) | Orchestrates a use case across ports. Owns the flow, not the rules or the wire format. |
| Domain / pure | service-owned domain packages, pure leaves (`internal/models`, `internal/fburl`, normalize/intent helpers) | Business rules + types. Pure: stdlib + `models` only. |
| Persistence / repository | `internal/store/*` | SQL + read-models. Owns rows; exposes typed methods. No transport, no handler types. |
| Drivers / adapters | `internal/drivers/*`, `internal/runtime/*` (CDP), embedding/HTTP clients | External-integration adapters behind a port. |

**Neutral leaf rule:** a package imported by many layers (`models`, `fburl`,
text-normalisation) must NOT import `services` / `server` / `store` / `drivers`.
A leaf that reaches upward stops being a leaf and creates a cycle.

## 2. Dependency-direction rules

- Transport MAY call application/usecase; it MUST NOT embed business rules.
- Application MAY depend on **narrow, consumer-owned ports/interfaces**, never on
  concrete server/transport types.
- Domain / pure helpers MUST NOT import transport / store / drivers.
- Store / drivers MUST NOT import server handlers.
- No `fiber` / `http` / `telegram` types inside domain or usecase packages unless
  explicitly approved (composition root adapts them).
- No raw `*Handler` and no raw `*sql.DB` / `*store.Store` crossing **into** a lower
  layer. A lower layer that needs a capability gets a narrow port, injected at the
  composition root.

Enforcement today is warn-only via `scripts/check_import_boundaries.sh` +
`scripts/check_topology.sh`; this playbook is the intent those guards encode.

## 3. Per-item migration playbook (feasibility before code)

Run this BEFORE branching. A precise "stopped / re-scoped" report is a successful
outcome (see `docs/ai/ESCALATION_PLAYBOOK.md`).

1. **Feasibility map first.** Read the cluster; list the functions/types involved.
2. **Receiver/coupling check.** Are the movers methods on a higher-layer type
   (`*Handler`)? Do they hold `*Handler` / raw DB / `*fiber.Ctx`? If yes, a package
   move needs a DI port → not move-only.
3. **Import-cycle check.** Would the destination import its parent (and vice-versa)?
   If yes, the move needs a deeper leaf or a port.
4. **Call-site count + public-API/export count.** How many sites change, and how
   many symbols must be exported? A "small facade" that needs 8 exports / 60 sites
   is not small — re-scope the estimate.
5. **Transport leakage.** Does the cluster carry `fiber`/`http`/`telegram` types
   that would leak into a domain package?
6. **Coverage / characterization.** Is the behavior covered? If not, add focused
   characterization tests before any move (move-only is not a licence to ship
   untested logic).
7. **Classify the lane** (below), then act — or stop and re-scope.

**Lanes:** GREEN = package-internal pure / file-responsibility cleanup, no
import-boundary or DB/auth/ledger/connector/queue/runtime change. YELLOW =
behavior-preserving move that crosses an import boundary (in Go a folder move *is*
an import-boundary change) or needs a narrow new port. RED = audit-only;
`status: BLOCKED`; human decision required; never auto-implemented.

**Same-package extraction is always allowed as prep** (it changes no imports). A
package/domain move happens **only when the boundary is already clean** — usually
after one or more prep extractions.

## 4. Boundary-target classification (queue field)

Every architecture queue item SHOULD carry a `boundary_target:` frontmatter value
naming the boundary move it serves (the queue INDEX lifecycle still owns
status/lane):

| `boundary_target` | Meaning | Typical lane |
|---|---|---|
| `prep-extraction` | Same-package responsibility/file split that prepares a later clean move. | GREEN |
| `leaf-move` | Move a pure/neutral cluster into its own leaf subpackage. | YELLOW |
| `transport-to-usecase` | Lift orchestration out of a transport/`*Handler` into an application/usecase package (usually needs a DI port). | YELLOW/RED |
| `store-test-seam` | Establish shared external-test scaffolding so per-domain store tests can move. | YELLOW |
| `blocked-decision` | Needs a founder/architecture decision before any code. | RED |

## 5. Reclassified blocked items

Current `status: BLOCKED` items, mapped to their boundary decision (item file
frontmatter remains the source of truth; `boundary_target` added there):

| Item | boundary_target | Open decision |
|---|---|---|
| ARCHCP3 | `leaf-move` | Move `intent` cluster to `copilot/intent/`. Decide whether the generic text-normalisation helpers (fold/strip/contains) belong in `intent.*` API or a separate `textnorm` leaf. ~8 exports / ~60 call sites. |
| ARCHSV2 | `transport-to-usecase` | Decouple `outboundFinalizer` from `*Handler` + `*fiber.Ctx` before any `finalize/` move; CAS-adjacent. Needs a DI port (RED-adjacent), not a move-only. |
| ARCHST1 | `store-test-seam` | Build a shared `storetest`-reachable seam (seeders + per-subpackage `newXStore`) before leads/coordination/outbound test moves; handle `db.db` raw access + cross-domain (RED-setup) tests. |
| ARCHSV-R2 | `blocked-decision` | `runtime_feed` handler is unwired dead code: delete vs wire the route — founder call (NOT a Sonar-cleanup decision). |
| ARCHCM-R1 | `blocked-decision` | RED controlled dependency: consolidate duplicated account-control RBAC; auth-critical, decision-record only. |

These stay BLOCKED until the named decision is taken; then the item is re-scoped
(corrected estimate + lane) and executed under §3.

## Validation / where this is enforced

- Import direction: `scripts/check_import_boundaries.sh` (warn) +
  `scripts/check_topology.sh` (executable invariants).
- File/responsibility size: `scripts/check_file_size.py`,
  `scripts/check_component_structure.py`.
- Per-item lifecycle: `docs/ai/AUTOPILOT_QUEUE.md` + `scripts/ai_queue_check.sh`.

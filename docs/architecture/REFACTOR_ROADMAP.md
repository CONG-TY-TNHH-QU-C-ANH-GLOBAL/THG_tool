# Refactor Roadmap

**Status:** OFFICIAL STANDARD (sequencing). **Companion of** `ARCHITECTURE_STANDARD.md`.
Staged path from the code-as-of-now (`CURRENT_CODE_AUDIT.md`) to the standard. Every
phase is **additive-first, behavior-preserving where possible, independently
revertible**. No big-bang. Product features (Phase H) come AFTER the boundaries and
the outbox exist, so they are built right the first time.

**Sonar cleanup work** (the `D.*` sprints below and all future ones) is governed by
the [**Sonar Factory Operating Protocol**](./SONAR_FACTORY_PROTOCOL.md) — risk
classes, batch budgets, controlled zones, and the agent workflow. This roadmap is
the *record*; that protocol is the *policy*.

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

### PR26B — Warn-only service-isolation boundary guards (`chore/pr26b-import-guard-service-isolation`)

Tooling-only extension of `scripts/check_import_boundaries.sh` (no script rewrite,
existing rules/output/known-gap behavior unchanged). Companion of
[`DIAGRAM_RECONCILIATION.md`](./DIAGRAM_RECONCILIATION.md) (PR26A) §11 / §6.

- **Scope:** `scripts/check_import_boundaries.sh` + this note only. CI untouched
  (already runs the script `|| true`, warn-only).
- **Rules added (3, all preventive, 0 warnings today):**
  - `SERVICE_NO_SIBLING` — generic guard: no `internal/services/<svc>` imports another
    sibling service (both directions; self-imports excluded). Broader than the
    facebook-specific `SERVICES_NO_SIBLINGS` (rule 7). Composition root (`cmd/*`) is not
    scanned — wiring services at `main` is allowed.
  - `WORKER_NO_TRANSPORT` — `cmd/worker` must not import HTTP/server transport
    (`internal/server*`, `internal/drivers/{http,telegram,connector}`).
  - `SIDECAR_NO_DIRECT_DB` — top-level `services/*` sidecars must not couple directly to
    the DB (`DATABASE_URL`/`DB_PATH`/`POSTGRES_*`/`sqlite3`/`psycopg`/`asyncpg`/
    `sqlalchemy`/`gorm`/`database/sql`); they must call a Go-owned versioned port.
- **Risk:** very low (tooling, warn-only; exit 0 preserved).
- **Behavior changed:** none (no runtime code/schema/migration/CI/package move).
- **Validation:** `bash scripts/check_import_boundaries.sh` → rules checked 13→16,
  warnings 4 (4 known, 0 other) unchanged, exit 0; `git diff --check`;
  `python scripts/check_file_size.py`; `go build ./...`; `go vet ./...`.
- **Rollback:** revert the script changes + this note.

### PR26C — Target boundary marker packages (`refactor/pr26c-arch-doc-markers`)

Move-only architecture scaffold: empty `doc.go` marker packages that make the
staged target structure physically visible. No runtime logic, no package moves, no
imports. Companion of [`DIAGRAM_RECONCILIATION.md`](./DIAGRAM_RECONCILIATION.md) §6.

- **Scope:** 8 new `doc.go` files + this note only.
- **Marker packages added (each: doc comment + `package` decl, nothing else):**
  - `internal/drivers/http` — inbound REST/SSE driver (today `internal/server/*`).
  - `internal/drivers/telegram` — inbound Telegram/webhook driver (today
    `internal/server/telegram` + `internal/telegram`).
  - `internal/drivers/connector` — inbound extension connector driver (today
    `internal/server/agent`); preserves pull-based outbox, no server-push.
  - `internal/services/taobao` — future Taobao vertical (resolver stub only today).
  - `internal/services/supplier1688` — future 1688 vertical; named `supplier1688`
    because a Go package identifier cannot begin with a digit (resolver stub
    `alibaba1688.go` today).
  - `internal/automation` — cross-vertical automation glue (must not become utils).
  - `internal/connectors` — connector domain/infra (today `store/connectors`,
    `browsergateway`, `cdpclient`, `local-connector-extension`).
  - `internal/crawler` — crawl execution (today `jobs`, `jobhandlers`, `store/crawl`,
    `cmd/worker`); must not import transport (`WORKER_NO_TRANSPORT`).
- **Behavior changed:** none (empty packages compile; nothing imports them).
- **Validation:** `gofmt -l` clean; `go list`/`build`/`vet`/`test ./...` pass;
  `bash scripts/check_import_boundaries.sh` unchanged (16 rules, 4 known / 0 other,
  exit 0); `python scripts/check_file_size.py` PASS; `git diff --check` clean.
- **Risk:** low (no-behavior scaffold).
- **Rollback:** delete the marker files + this note.

### PR26F — Module ownership / inventory sync (`docs/pr26f-module-ownership-sync`)

Docs/metadata-only sync of the ownership manifest + inventory with the PR26C marker
packages; no schema rewrite, no guard changes, no Go files touched.

- **Scope:** `MODULE_OWNERSHIP.yml`, `CURRENT_PACKAGE_INVENTORY.md`, `MODULE_BOUNDARIES.md`,
  `DIAGRAM_RECONCILIATION.md`, this note only.
- **Synced:** added `MODULE_OWNERSHIP.yml` entries for `drivers/http`, `drivers/telegram`,
  `drivers/connector`, `connectors`, `crawler`, `automation` (status `scaffolded` = doc.go
  marker exists, per the file header enum); flipped `services/taobao` + `services/1688`
  from `documented` → `scaffolded`. Added a marker table + 1688-naming note to
  `CURRENT_PACKAGE_INVENTORY.md`.
- **Canonical 1688 naming:** Go path `internal/services/supplier1688` (a package name
  cannot start with a digit; `internal/services/1688` is invalid); product label "1688";
  resolver stub `internal/platform/services/resolver/alibaba1688.go` NOT renamed. Fixed the
  stale `target_paths: internal/services/alibaba1688` → `supplier1688` and the
  `internal/services/{taobao,1688}` path ref.
- **Behavior changed:** none (docs/metadata only; existing YAML schema/style preserved;
  known-gap entries kept; no guard made stricter).
- **Fix-up:** corrected a `cmd/agent` documentation overclaim in `DIAGRAM_RECONCILIATION.md`
  (§2 + §6 target tree). `cmd/agent` is **planned/aspirational only** — it has **no tracked
  Go package and no committed history** (`git ls-files cmd/agent` empty; `git log --all --
  cmd/agent` empty; `go build ./...` green without it). Do not document it as existing
  runtime code.
- **Workspace-cleanup caution (future prompts):** before any `git clean -fd`, run
  `git clean -nd` first (dry-run) and **stop** if source-like untracked dirs appear —
  e.g. `cmd/*`, `internal/*`, `services/*`, `openspec/*`, `specs/*`, `docs/*`,
  `frontend/src/*`, or `local-connector-extension/*`. An untracked `cmd/agent/` was lost
  this way; untracked source dirs are not git-recoverable.
- **Validation:** `git diff --check`; `check_file_size.py`; `check_import_boundaries.sh`
  (16 rules, 4 known / 0 other, exit 0 — unchanged); `go list/build/vet ./...`.
- **Rollback:** revert the docs edits + this note.

### PR26H — Phase C move gate: isolate the Facebook lead action seam (`refactor/pr26h-phase-c-facebook-lead-seam`)

First Phase C **production-code** step after the PR26E characterization tests. Outcome **A**
(safe direct extraction) — no port needed because the only caller is `cmd/scraper`
(`package main`, the composition root), which may import a service package.

- **Seam isolated:** the pure Facebook-specific synthetic-lead shaping (the `prompt_target`
  convention: `Platform=Facebook`, `SourceType="prompt_target"`, comment `post_url`→`SourceURL`
  / other `target_url`→`AuthorURL` field mapping) moved out of
  `cmd/scraper/outbound_actions.go` (`leadsFromActionArgs`) into
  `internal/services/facebook.SyntheticLeadFromActionArgs` (pure, imports only `models`).
- **Stayed neutral in `main`:** the store-dependent orchestration (`lead_id` lookup,
  `WorkQueueLeads` fallback) — not FB-specific — remains in `leadsFromActionArgs`, now
  delegating the FB shaping.
- **Import direction:** `cmd/scraper (main / composition root) → internal/services/facebook
  → internal/models`. Preferred direction; no neutral package imports the FB service; no cycle.
- **Behavior:** identical (pinned by `cmd/scraper/leads_from_action_args_test.go` +
  `internal/services/facebook/lead_action_args_test.go`). No port, no CommandBus, no registry,
  no DI singleton; `queueLeadOutreach`/`commentSinglePost`/approval-default/copy unchanged.
- **Validation:** `go test/build/vet ./...` pass; `check_import_boundaries.sh` 16 rules,
  4 known / 0 other, exit 0 (unchanged); `check_file_size.py` PASS; `git diff --check` clean.
- **Rollback:** inline the FB function back into `leadsFromActionArgs`, delete the new files.
- **Next:** further FB workflow surface (crawl handler / `queueLeadOutreach` FB target-URL
  resolution) is a larger Phase C move, still gated on tests + review.

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

### D.1 — Sonar cognitive-complexity hygiene, batch 2 (refactor-only)  ✅ DONE

Second one-issue hygiene batch, same doctrine as D.0 (no Phase D redesign; local
extraction only). Isolated PR off `main` after D.0 (Batch 1) merged via PR #12.

- **Branch:** `refactor/sonar-cognitive-batch-2` (from `origin/main` @ `f7cb2c7b`).
- **Sonar issue/rule fixed:** `AZ7askWv1xM_XIKj2DHJ` — `go:S3776` (cognitive
  complexity 22 → ≤15) on `cmd/scraper/crawl_runtime.go:19` `submitOpenCrawl`.
- **Changed files:** `cmd/scraper/crawl_runtime.go` (this `REFACTOR_ROADMAP.md` note).
- **What changed:** extracted four pure-local private helpers from the arg-resolution
  prefix of `submitOpenCrawl` — `resolveCrawlMaxItems`, `resolveCrawlKeywords`,
  `resolveCrawlAccountID`, `buildCrawlExtras`. The `jobs.Task` literal and the
  submit/route tail (`submitConnectorCrawl` / `jobStore.Submit`) are untouched.
- **Refactor-only or behavior-changing:** **refactor-only.**
- **Behavior preserved:** yes — identical fallback chains, the `args["account_id"]`
  write-back on auto-pick, `Task` fields, `context`/error strings, logs, and call
  order. No connector claim / CAS / lease / ledger / outbound hot-path logic touched
  (those live in `submitConnectorCrawl`/`enqueueConnectorCrawlCommand`, unchanged).
  `org_id` tenant scoping preserved. No package move, no new import, no new boundary.
- **Validation:** `gofmt` clean; `go vet ./...` clean; `go build ./...` clean;
  `go test ./...` PASS (`cmd/scraper` ok); `-race` not runnable in this Windows env
  (`CGO_ENABLED=0`, no C compiler — leave to CI/Linux); `check_import_boundaries.sh`
  exit 0 (4 pre-existing known-gap warnings, **0 new**); `check_file_size.py` PASS;
  `git diff --check` clean.
- **Remaining risks:** none identified (mechanical move; same package, same calls).
- **Remaining Sonar `go:S3776` backlog (cmd/scraper):** `crawl_runtime.go:114 (29)`
  `pickReadyFacebookAccountIDForCrawl` (connector-readiness — higher risk),
  `main.go:28 (64)`, `outbound_actions.go:90 (95)/698 (19)/806 (33)` (outbound hot
  path — gated on characterization tests, Phase C/I), `skills_register.go (22)`; plus
  ~35 more across `internal/{drivers/copilot,leadingest,server,store,ai}` — one batch
  at a time after review.

### D.2 — Sonar cognitive-complexity hygiene, batch 3 (refactor-only)  ✅ DONE

Third one-issue hygiene batch, same doctrine as D.0/D.1 (no Phase D redesign; local
extraction only). Isolated PR off `main` after D.1 (Batch 2) merged via PR #13.

- **Branch:** `refactor/sonar-cognitive-batch-3` (from `origin/main` @ `3b4fff5e`).
- **Sonar issue/rule fixed:** `AZ7askww1xM_XIKj2DTh` — `go:S3776` (cognitive
  complexity 17 → ≤15) on `internal/server/leads/lifecycle.go:20`
  `getLeadLifecyclesBatch` (read-only `GET /api/leads/lifecycle?ids=` projection).
- **Changed files:** `internal/server/leads/lifecycle.go` (this `REFACTOR_ROADMAP.md` note).
- **Risk level:** **LOW** — drivers/http transport handler; read-only endpoint, no DB
  writes, no outbound / connector claim-CAS-lease / ledger-execution_attempts / policy /
  auth-logic / migration touched.
- **Refactor-only or behavior-changing:** **refactor-only.**
- **What changed:** extracted the comma-separated `?ids=` parsing/validation loop into a
  same-package private helper `parseLeadLifecycleIDs(raw) ([]int64, error)`. The handler
  keeps the `org_id` guard, the empty-`raw` 200 early-return, the store call, and the
  500 path verbatim.
- **Behavior preserved:** yes — identical 100-id cap, empty-segment skipping, and id
  validation; the helper returns errors whose `.Error()` strings match the previous 400
  bodies exactly (`max 100 ids per call`, `invalid id: <p>`), so the wire contract is
  unchanged. `org_id` tenant scoping untouched. One new stdlib import (`errors`) — chosen
  over `fmt.Errorf` to avoid a `go vet` non-constant-format-string warning.
- **Validation:** `gofmt` clean; `go vet ./...` clean; `go build ./...` clean;
  `go test ./...` PASS (`internal/server/leads` ok); `-race` not runnable in this Windows
  env (`CGO_ENABLED=0`, no C compiler — leave to CI/Linux); `check_import_boundaries.sh`
  exit 0 (4 pre-existing known-gap warnings, **0 new**); `check_file_size.py` PASS;
  `git diff --check` clean.
- **Remaining risks:** none identified (mechanical move; same package, same calls).
- **Remaining Sonar `go:S3776` backlog (low-risk non-test pool, cx asc):**
  `internal/server/org/handlers.go:289 (16)`, `internal/server/org/superadmin.go:160 (16)`,
  `internal/server/leads/engagement.go:46 (17)`, `internal/server/workspace/watchers.go:331 (17)`,
  `internal/server/org/users.go:14 (18, auth — handle with care)`, plus ~100 more across
  cmd/server/store/ai — one batch at a time after review.

### D.3 — Sonar low-risk cleanup sprint 1 (refactor-only)  ✅ DONE

First **risk-lane sprint** (the doctrine shift from "one issue per PR" to "one
low-risk lane per PR"): multiple `go:S3776` issues fixed in one branch/PR because
they share one provably behavior-free category — **read-only GET projection
handlers in `internal/server`**. Same mechanical-extraction doctrine as D.0/D.1/D.2;
no Phase D redesign.

- **Branch:** `refactor/sonar-low-risk-sprint-1` (from `origin/main` @ `ffe31390`,
  the Batch-3/PR#14 merge).
- **Lane:** **B — low-risk local handler refactors.** Lane A (2 trivial
  `docker:S7031` "merge consecutive RUN" on the root `Dockerfile`) was deferred as
  too small per the planner rule.
- **Sonar issues/rule fixed (3 × `go:S3776`):**
  - `AZ7askxg1xM_XIKj2DTx` — `internal/server/observability/handlers.go:84`
    `executionRecent` (cx 21 → ≤15), `GET /api/observability/execution/recent`.
  - `AZ7askxZ1xM_XIKj2DTv` — `internal/server/observability/runtime_feed.go:43`
    `runtimeFeed` (cx 27 → ≤15), `GET /api/observability/runtime-feed`.
  - `AZ7askw11xM_XIKj2DTi` — `internal/server/leads/engagement.go:46`
    `getLeadEngagementsBatch` (cx 17 → ≤15), `GET /api/leads/engagement?ids=`.
- **Changed files:** `internal/server/observability/handlers.go`,
  `internal/server/observability/runtime_feed.go`, `internal/server/leads/engagement.go`,
  `internal/server/leads/lifecycle.go` (helper rename only), and this note.
- **Risk level:** **LOW** — all three are read-only HTTP projection handlers; no DB
  writes, no auth/admin, no connector claim-CAS-lease, no ledger/execution_attempts
  *logic* (they only project those rows read-only), no policy/readiness, no outbound
  spine, no migration.
- **Refactor-only or behavior-changing:** **refactor-only.**
- **What changed:** extracted each handler's per-row mapping loop body **verbatim**
  into a same-package private helper — `buildRecentAttemptRow(models.ExecutionAttempt)`
  and `buildRuntimeFeedRow(coordination.RuntimeEvent)` (both element types already
  imported by their packages; `runtime_feed.go` gained the already-in-package
  `coordination` import, **no new dependency edge** — import-guard reports 0 new
  warnings). For `getLeadEngagementsBatch`, the duplicated `?ids=` parse loop was
  replaced by the **shipped Batch-3 helper** (DRY): `parseLeadLifecycleIDs` was renamed
  to the endpoint-neutral `parseLeadIDsCSV` and reused by both batch endpoints.
- **Behavior preserved:** yes — identical status codes (400/403/500), JSON keys/shape,
  error strings (`max 100 ids per call`, `invalid id: <p>`), `time.RFC3339` formats,
  evidence/attrs JSON parsing, query-param clamps, and the `org_id` guards verbatim.
  The leads parser reuse is exact (same cap/skip/reject + identical 400 bodies).
- **Validation:** `gofmt` clean; `go vet ./...` clean; `go build ./...` clean;
  `go test ./...` PASS (full suite; `internal/server/leads` ok, observability has no
  test files); `-race` not runnable in this Windows env (`CGO_ENABLED=0`, no C
  compiler — leave to CI/Linux); `check_import_boundaries.sh` exit 0 (4 pre-existing
  known-gap warnings, **0 new**); `check_file_size.py` PASS (0 new oversized);
  `git diff --check` clean. The `specs/RETRIEVAL_SOAK_REPORT.md` rewrite that
  `go test ./...` produces was reverted, not staged.
- **Fix-up (new-code duplication):** the first Sonar PR scan failed the gate on
  Duplicated Lines on New Code (5.63% > 3.0%, = 4 new lines). Cause: reusing
  `parseLeadIDsCSV` made `getLeadEngagementsBatch`'s `?ids=` prologue token-identical
  to `getLeadLifecyclesBatch`'s, so CPD flagged the shared block. Fix: extracted that
  prologue (org guard + empty-`ids` early return + parse) into one same-package helper
  `leadBatchIDsFromQuery(c, emptyKey)` used by both batch handlers; removed the now-
  unused `strings` import from `engagement.go`. Behavior/wire bodies identical (same
  400/200 status, same empty-map keys `engagements`/`lifecycles`, same error strings).
  Expected new-code duplication after re-scan: 0%. No new files; no scope change.
- **Remaining risks:** none identified (mechanical verbatim moves; same packages,
  same calls).
- **Remaining Sonar `go:S3776` backlog by lane (after this sprint):** Lane A — 2
  `docker:S7031` (root `Dockerfile`). Lane B — remaining read-only handlers, plus
  higher-cx server handlers (`leads/handlers.go:116 (55)`, `workspace/handlers.go:27 (33)`,
  `knowledge/handlers.go:174 (26)`) deferred for individual review. Lane D/E (excluded
  here) — `org/{handlers,users,superadmin,identity}.go` (auth/admin/connector binding),
  `workspace/{watchers,handlers,screen_proxy}.go` (CDP/connector), `crawl/intent.go`
  (job submission), `system/notifications.go` (outbound Report). ~100 more across
  cmd/store/ai/copilot — one safe lane at a time after review.

### D.4 — Sonar cleanup sprint 2 (refactor-only)  ✅ DONE

Second risk-lane sprint, **Lane C** (local helper extraction). Bulk-fixed five
`go:S3776` cognitive-complexity issues that share one provably behavior-free
category — **pure-compute / read-model functions** (KnowledgeOS governance,
assembly, ingestion, products + one read-only store aggregation). Same
mechanical-extraction doctrine as D.0–D.3; no Phase D redesign.

- **Branch:** `refactor/sonar-cleanup-sprint-2` (from `origin/main` @ `86e6b66f`,
  the Sprint-1/PR#15 merge).
- **Lane / agents (simulated — named subagents not available in this env, roles
  labelled explicitly):** Lane C — `/senior-architect` + `/senior-backend` +
  `/code-reviewer` (+ `/qa-test-engineer`). `/security-review` confirmed **no**
  security-relevant code is touched.
- **Sonar issues/rule fixed (5 × `go:S3776`):**
  - `AZ7askjz1xM_XIKj2DP5` — `workspace_knowledge/governance/output_validator.go:102`
    `ValidateOutput` (cx 23 → ≤15); extracted `bannedClaimReasons` + `fabricatedPriceReasons`.
  - `AZ7askl91xM_XIKj2DQc` — `workspace_knowledge/assembly/context_assembly.go:136`
    `renderProduct` (cx 19 → ≤15); extracted `appendStructuredProductParts`.
  - `AZ7askjf1xM_XIKj2DP2` — `workspace_knowledge/ingestion/csv/columns.go:129`
    `toAsset` (cx 23 → ≤15); extracted `rowPayloadJSON`.
  - `AZ7askl11xM_XIKj2DQb` — `workspace_knowledge/products/canonical.go:202`
    `Validate` (cx 31 → ≤15); extracted `validateHeader` + `validateVariant`.
  - `AZ7askoB1xM_XIKj2DQ4` — `store/knowledge/cost.go:143` `ListOrgsByEmbeddingCost`
    (cx 18 → ≤15); extracted `accumulateOrgEmbeddingCost` + `sortOrgEmbeddingCostByTokensDesc`.
- **Changed files:** the five files above + this note.
- **Risk level:** **LOW** — four are pure functions (no IO); the fifth is a read-only
  `SELECT` aggregation (explicitly cross-org superadmin, no per-tenant gate). None
  touch outbound spine / connector claim-CAS-lease / ledger-execution_attempts /
  policy-readiness / auth-admin / migrations / `internal/ai`.
- **Refactor-only or behavior-changing:** **refactor-only.**
- **Behavior preserved:** verbatim moves — identical validation verdicts/reason codes
  and order, identical rendered product strings + ` · ` separators, identical CSV
  payload JSON, identical error strings/order, identical SQL + O(n²) sort tie-order +
  limit. The two validator helpers take pointers to slice elements so the in-place
  `Availability=AvailUnknown` defaulting still mutates the originals.
- **Validation:** `gofmt` clean; `go vet ./...` clean; `go build ./...` clean;
  `go test ./...` PASS (changed-package fixtures — governance, products' 7 fixtures,
  csv, assembly, store/knowledge — all green); `-race` not runnable in this Windows
  env (`CGO_ENABLED=0`, no C compiler — leave to CI/Linux); `check_import_boundaries.sh`
  exit 0 (4 pre-existing known-gap warnings, **0 new**); `check_file_size.py` PASS;
  `git diff --check` clean. Soak-report test side-effect reverted, not staged.
- **Code-reviewer result:** PASS (no medium/high-risk file changed; mutation semantics,
  error strings, ordering all preserved). Watch-item: confirm new-code duplication
  stays ≤3% on the next Sonar PR scan (helper shapes similar but bodies non-identical).
- **Remaining risks:** none identified (mechanical verbatim moves; same packages).
- **Deferred security proposal (Lane S, NOT edited):** the 14 `go:S2092` "cookie
  missing Secure flag" vulnerabilities live in `internal/server/auth/handlers.go` (11)
  + `internal/server/org` (3). Setting `Secure: true` is auth-sensitive and
  behavior-changing (breaks non-HTTPS/dev). Proposal: gate the flag on an env/config
  (`Secure: cfg.CookieSecure`, default true in prod) behind a characterization-test-first
  plan + `/security-review` approval — a future Lane S sprint, not this one.
- **Remaining Sonar backlog:** ~1276 open (Maintainability 1172, Reliability 122,
  Security 15). `go:S3776` now ~104 (mostly Lane D/E: agent/workspace/auth/copilot/
  cmd-scraper). Other safe Lane C pure-compute `go:S3776` remain in
  `workspace_knowledge/{retrieval,ingestion/rest_json}` + `store/{leads,knowledge}` for
  a future batch.

### D.5 — Sonar cleanup sprint 3 (refactor-only)  ✅ DONE

Third risk-lane sprint, **Lane F** (frontend-only low-risk), Sprint Option 3 —
a high-throughput bulk-safe sweep of one provably zero-runtime rule family,
**`typescript:S6759` ("React component props should be read-only")**. Fix is a
pure compile-time annotation: each component's props parameter type wrapped in
`Readonly<…>`. Emitted JS is byte-identical. Pivoted off the Go lanes (D.0–D.4
already harvested the safe `go:S3776`/helper backlog; remaining Go issues sit in
Lane D/E controlled zones).

- **Branch:** `refactor/sonar-cleanup-sprint-3` (from `origin/main` @ `3f382ac5`,
  the `.claude/agents` framework PR #17 merge).
- **Lane / agents (direct role execution — the named project subagents under
  `.claude/agents/` were dropped by the runtime mid-session, so each role's
  checklist was executed inline and labelled explicitly):** Lane F —
  `senior-architect` (gate: ALLOW WITH CONDITIONS) + `senior-frontend` (implement)
  + `code-reviewer` (final gate) + `qa-test-engineer` (validation). `sonar-triage`
  produced the inventory + lane classification.
- **Sonar issues/rule fixed (26 × `typescript:S6759`, 22 files):** all in
  `frontend/` presentational React components — `components/data/` (9:
  BusinessMemoryPanel ×2, ConfidenceTag, ContextSummaryPanel, DataSourcesPanel,
  DataStatsGrid, MagicOmnibox, OutboundPolicyPanel, PrivateFilesTable),
  `frontend/components/` (5: StatusBadge ×2, KPICard, ProgressBar, PageWrapper),
  `src/marketing/` (5: PlatformLanding ×3, ComingSoonLanding, MarketingNav),
  plus ActorVerdictChip, InviteNotificationCard, LifecycleTabs,
  FacebookProductLanding, AccountHealthCard, AccountHealthBoard, NextStepsPanel.
- **Changed files:** the 22 `.tsx` files above + this note.
- **Risk level:** **LOW** — `Readonly<T>` is erased at emit and marks only the
  props object's own properties immutable (no deep array conversion), so it cannot
  compile-fail unless code reassigns a prop (none do; verified). None touch outbound
  spine / connector claim-CAS-lease / ledger / policy-readiness / auth-admin /
  migrations / workspace-switching. `OutboundPolicyPanel` is presentational (calls
  injected `onChange` only); the spine stays server-side.
- **Refactor-only or behavior-changing:** **refactor-only.**
- **Behavior preserved:** no JSX, copy, `className`, route, API call, client state,
  effect-dependency, or event-handler changed. Diff is exactly 26 added / 26 removed
  lines, every added line a `Readonly<…>` wrap.
- **Excluded controlled zones:** `internal/server/auth|org/*` cookie code
  (14 × `go:S2092`, Lane S∩E), migrations (`plsql`, Lane E), `local-connector-extension/*`
  (Lane E), all `views/*` + `JoinWorkspace`/`FileUploadPanel` carrying deferred
  `S2871` sort / `S1082` a11y / `S3923` / `S1763` reliability bugs (Lane R, test-first).
- **Validation:** `next build` ✓ compiled + "checking validity of types" PASS (no
  type errors); `next lint` **not configured** (drops to interactive setup) — noted,
  not run; `check_file_size.py` PASS (0 new oversized; `PlatformLanding.tsx` is an
  allowlisted legacy 800-line file, touched at +0 net lines — annotation can't be
  extracted, did not grow); `git diff --check` clean; `.mcp.json` untracked, not
  staged; no build/coverage artifacts staged (`.next/` git-ignored).
- **Code-reviewer result:** PASS (every change maps to a selected S6759 issue; no
  controlled file; no runtime/UX/API change; no duplication risk — annotations, not
  shared helpers).
- **Remaining Sonar backlog:** ~1250 open after this sweep. `typescript:S6759` drops
  ~106 → ~80 (remainder in `views/`, auth, telegram-connect, workspace-switch files
  excluded here). Reliability (122) + Security (15) untouched — deferred to Lane R
  (test-first) / `security-review` (the `go:S2092` cookie proposal from D.4).

### D.6 — Local connector pairing reliability, sprint 4 (behavior-changing)  ✅ DONE

Not a Sonar sweep (D.0–D.5 were). This sprint fixed a **customer-blocking reliability
bug**: the THG Chrome extension pairing got stuck on **"Verifying…" forever** after
the operator entered a dashboard pairing code (e.g. `3CEK-7K8P`), never advancing to
Facebook login / ready. Extension-only fix; backend pairing endpoints were inventoried
and found correct + secure, so they were left untouched.

- **Branch:** `fix/local-connector-pairing-sprint-4` (from `origin/main` @ `fd56e86a`).
- **Lane / agents:** `senior-frontend` + `senior-backend` + `senior-fullstack`
  (3-layer pairing-flow inventory), `sonar-triage` (focused connector scan),
  `qa-test-engineer` (repro + tests), `senior-architect` (gate: **ALLOW WITH
  CONDITIONS**), `security-review` (**CONFIRM**), `code-reviewer` (final gate: **APPROVE**).
- **Root cause:** the extension pairing path left an awaited promise unsettled, so the
  popup's button-reset `finally` (`popup.js:116-119`) never ran:
  1. **No fetch timeout** — bare `fetch` at `src/api.js` pair POST + `agentFetch`. A
     stalled/black-holed request never resolves or rejects.
  2. **Pairing blocked on a full heartbeat** — `pairConnector` did `await
     THGHeartbeat.run()` *after* the device token was already stored, so even a
     server-side-successful pair stayed "Verifying…" until/unless an un-timed
     heartbeat/stream fetch chain completed (it violated heartbeat.js's own documented
     "liveness must not depend on heavy work" decoupling).
- **Fix:** added generic `THGShared.fetchWithTimeout` (AbortController, 20s, **no
  retry** — a consumed code must never be replayed); routed the pair POST + `agentFetch`
  through it with a **static** Vietnamese timeout/network message (no code/token
  interpolation); changed the post-pair heartbeat to fire-and-forget
  `THGHeartbeat.run().catch(() => {})` so pairing returns the instant the token is
  stored. Liveness is unaffected — it is driven by the 30s alarm armed in
  `background.js` (`THGHeartbeat.schedule()`), not by this call.
- **Changed files:** `local-connector-extension/src/shared.js` (+`fetchWithTimeout`,
  131 lines), `src/api.js` (134 lines), `manifest.json` (version `0.5.56→0.5.57`), new
  `src/pairing_reliability.test.mjs` (regression net incl. the fire-and-forget liveness
  guard), + this note.
- **Refactor-only or behavior-changing:** **behavior-changing** (reliability fix; tests
  added).
- **Security / tenant:** no Facebook password stored (only `deviceToken`/`connectorId`
  session identity persisted); no auth/CORS/manifest-permission change; pairing code
  stays one-time / org-scoped / expiring (server-side, untouched); no secrets logged.
- **Excluded controlled zones (untouched):** backend pairing/claim/CAS/lease,
  `action_ledger`/`execution_attempts`, outbound action execution, policy/readiness
  gates, auth/admin, migrations, dashboard wizard. Dashboard auto-poll of
  `facebook-status` (the wizard advances optimistically with no poll) was **deferred**
  to a separate follow-up PR per architect (track separation; `FacebookConnectionWizard.tsx`
  already 177 lines — the poll belongs in its own hook).
- **Validation:** `node --test local-connector-extension/src/` → **29 pass / 0 fail**
  (incl. 5 new pairing assertions); `python scripts/check_file_size.py` PASS (api.js/
  shared.js ≤ 200); `git diff --check` clean; `.mcp.json` untracked, not staged; no
  generated artifacts staged.
- **Manual E2E (acceptance):** open dashboard ws_5 → generate code → paste into extension
  → Verify completes within timeout and the popup leaves "Verifying…"; an invalid/expired
  code or a stalled network shows a clear Vietnamese error within ~20s instead of hanging;
  no Facebook password requested or stored.
- **Customer impact:** operators can pair the local connector again; failures surface a
  clear, code-safe error instead of an infinite spinner.
- **Remaining risk:** dashboard still requires a manual "kiểm tra" click at step 3 (no
  auto-poll yet — deferred follow-up). A wrong/stale Advanced "Server URL" pointing off
  `*.thgfulfill.com` is an environment/config issue, now surfaced as a timeout error
  rather than a hang.

### D.7 — Comment AI sales-contact resolution, sprint 5 (behavior-changing)  ✅ DONE

Customer bug: a sales agent configured their own Comment-AI contact ("Liên hệ của
tôi trong comment AI" — Telegram @hairypotter98 · Zalo 0949716391, CTA set) and the
preview was correct, but generated comments still cited the **company/workspace
contact**. UI promise: *"Khi lead do bạn phụ trách, AI dùng liên hệ này thay cho liên
hệ chung của công ty."* Backend-only fix; no migration, no schema, no frontend.

- **Branch:** `fix/comment-ai-sales-contact-sprint-5` (from `origin/main` @ `35fbf9aa`,
  the Sprint-4 merge).
- **Lane / agents:** `senior-frontend` + `senior-backend` + `senior-fullstack`
  (3-layer contact-flow inventory), `sonar-triage` (focused scan), `senior-architect`
  (gate: **ALLOW WITH CONDITIONS**, confirmed Contract A), `security-review`
  (**CONFIRM**), `qa-test-engineer` (8 precedence tests), `code-reviewer` (**APPROVE**).
- **Root cause:** `resolveStaffContactIdentity` (`cmd/scraper/outbound_contact_identity.go`)
  keyed the staff contact ONLY on the executing FB account's `AssignedUserID`, ignoring
  `actx.InitiatorUserID` (the member who owns the execution = `created_by` — the agent
  handling this outreach). Leads are SHARED (no per-lead owner column;
  `feedback_shared_battlefield_not_crm.md`), so the initiator is the correct subject for
  "lead do bạn phụ trách". When the executing account was unassigned (or assigned to
  someone else), David's saved profile (keyed by his user_id) was never read → company
  fallback won.
- **Fix (Contract A precedence, first usable wins):** initiating sales agent
  (`InitiatorUserID`) → executing account's assignee (`AssignedUserID`) → company default
  (only if `companyContactFallbackAllowed`) → omit. A profile is "usable" only when
  `Active && ContactLine() != ""`, so an empty/inactive profile falls through (this makes
  initiator-first degrade safely to the assignee for a contactless admin). Added pure
  helper `usableStaffContact`; reused the existing pure `models.ApplyStaffContact` for the
  final staff-or-fallback/omit decision (never invents contact data).
- **Changed files:** `cmd/scraper/outbound_contact_identity.go` (resolver, 68 lines),
  `cmd/scraper/outbound_actions.go` (one-line call-site change passing
  `actx.InitiatorUserID`), new `cmd/scraper/outbound_contact_identity_test.go` (8 tests),
  + this note.
- **Refactor-only or behavior-changing:** **behavior-changing** (contact selected
  differently; tests added). Track: SaaS UX Hardening / staff-contact (PR-5 extension).
- **Security / tenant:** reads stay org-scoped (`GetStaffContactProfile` is
  `WHERE user_id=? AND org_id=?`; `GetAccountForOrg` rejects other orgs). `InitiatorUserID`
  is server-derived (JWT → forcibly overwritten in `skills_register.go:243`), not
  request-spoofable. No cross-tenant / cross-staff leak, no auth change, no contact PII
  logged, no invented data.
- **Excluded controlled zones (untouched):** Facebook write execution, queue/policy/
  readiness gates, `action_ledger`/`execution_attempts`, connector claim/CAS/lease,
  schema/migrations, frontend (the form was already correct). The outbound safety spine
  is untouched — only the grounded contact string changes.
- **Validation:** `go build ./...` ✓, `go vet ./...` ✓, `go test ./...` ✓ (8/8 new
  precedence subtests pass; soak report reverted to keep tree clean); `gofmt` content
  clean (repo CRLF checkout flags `gofmt -l` repo-wide — environment artifact);
  `check_file_size.py` PASS (resolver 68 ≤ 200); `git diff --check` clean; `.mcp.json`
  untracked, not staged.
- **Manual E2E:** configure staff contact (Telegram @hairypotter98 · Zalo 0949716391,
  empty phone/email, CTA) → generate a Comment-AI draft as that agent → draft cites the
  staff Telegram/Zalo + staff CTA, not the company contact, with empty fields filtered;
  an agent with no usable profile falls back to the account assignee, then company (if
  policy allows); another staff's contact never leaks into this agent's draft.
- **Remaining risk / deferred:** the `reasoning=live` path (`GenerateCommentV2`,
  default-off) still re-derives company identity internally and drops the staff swap —
  a separate, default-off latent bug **deferred** to a follow-up (track separation;
  needs its own live-path test). This PR does not make that gap worse.

### D.8 — Comment AI contact identity parity fix (behavior-changing)  ✅ DONE

Follow-up to D.7. Manual testing showed two remaining defects plus the D.7-deferred
`reasoning=live` bug. Closes all three so the staff/company contact contract is
consistent across BOTH comment generation paths.

- **Branch:** `fix/comment-ai-contact-website-live-path` (from merged main @ `372a629c`).
- **Lane / agents:** `senior-fullstack`/`senior-backend` (re-map), `sonar-triage`
  (input), `senior-architect` (**ALLOW WITH CONDITIONS**), `security-review`
  (**CONFIRM**, C1–C3), `qa-test-engineer` (contract tests), `code-reviewer` (**APPROVE**).
- **Root causes (3):**
  1. **`reasoning=live` bypass (data):** `buildGroundedCommentPrompt` re-derived a
     company-only identity via `ResolveCompanyIdentity`, dropping the staff-contact
     swap — so live mode cited the company contact even when a staff contact existed.
  2. **Company website dropped when staff contact wins (prompt):** `CompanyIdentity`
     already separates `Website` from `OfficialContact` and `ApplyStaffContact`
     preserves `Website`, but `buildContactRule`/msggen rule 6 framed the website as
     optional and lumped it with contacts ("at most one URL, no spam"), so a rich
     staff contact crowded out the URL. The guard never re-adds it.
  3. **Resolved CTA never rendered:** `id.PrimaryCTA` (staff or company) was resolved
     but not rendered in either prompt, so the staff CTA never appeared.
- **Contract:** Contact channels (phone/Telegram/Zalo/email/CTA) follow staff →
  account assignee → company (if allowed) → omit. Company WEBSITE is always included
  when configured, INDEPENDENT of the contact-channel precedence. Never invent; empty
  fields omitted.
- **Fix:** new shared `resolveCommentIdentity` (`cmd/scraper/outbound_contact_identity.go`)
  = `ai.ResolveCompanyIdentity` + `resolveStaffContactIdentity`, used by BOTH paths
  (normal with `groundedCTA=nil`, live with `decision.Selected.CTA`) — one precedence
  path. `GenerateCommentV2`/`buildGroundedCommentPrompt` now take the resolved
  identity (no internal re-derive). `buildContactRule` makes the website MUST-include
  EXACTLY ONCE and independent of the contact line (only when `Website != ""`; empty
  still says "do NOT include any URL"). CTA single-sourced via
  `ctaSuffix(identity.PrimaryCTA)` in both paths (`buildCompanyBlock` stays facts-only
  → no double-CTA). `applyCommentReasoning` threads `actx.InitiatorUserID`.
- **Changed files:** `cmd/scraper/outbound_contact_identity.go` (+shared helper),
  `cmd/scraper/outbound_actions.go` (call swap + live threading),
  `internal/ai/comment_decision.go` (signatures + `buildContactRule` + `ctaLine`),
  `internal/ai/msggen.go` (rule 6), 2 fixed + 2 new test files, this note.
- **Refactor-only or behavior-changing:** **behavior-changing** (grounded prompt
  content changes; tests added). Track: Comment Intelligence.
- **Security / tenant:** reads org-scoped; `InitiatorUserID` server-derived (not
  spoofable); resolved identity stays an in-memory prompt input — NOT added to
  `CommentDecision`, logs, or `InsertSystemPromptLog`; website MUST-include is
  conditional on a configured website (no invented URL).
- **Excluded controlled zones (untouched):** Facebook write execution, queue/policy/
  readiness gates (`EvaluateGate`/`ApplyGate`), `action_ledger`/`execution_attempts`,
  connector claim/CAS/lease, migrations, frontend. The outbound safety spine is
  untouched — only the grounded prompt identity content changes.
- **Validation:** `go build ./...` ✓, `go vet ./...` ✓, `go test ./...` ✓ (new
  resolver-parity + prompt-wording tests pass; soak report reverted); `check_file_size.py`
  PASS (net-new logic in the 82-line resolver file); `git diff --check` clean;
  `.mcp.json` untracked, not staged.
- **Manual E2E:** with staff Telegram/Zalo + company website configured, a draft cites
  the staff Telegram/Zalo + staff CTA AND the company website (no company hotline/email);
  staff-phone case uses staff phone + website; empty staff contact falls back to company
  + website; `reasoning=live` produces the same contact result as the normal path.
- **Deterministic website guard (same-branch fix-up):** prompt instruction alone could
  still be omitted by the model, so `comment.EnsureWebsite` (`internal/ai/comment/contact_grounding.go`)
  now makes the configured company website the SINGLE, preferred/required URL of the
  final draft under the ≤1-URL policy — grounded-only (only ever emits
  `CanonicalWebsite(id.Website)`, never invents). It delegates to `RepairCommentContacts`
  (t.me→@handle, normalize website variants, dedupe, strip non-grounded) and then: if the
  website is present it is the one canonical URL; if a competing link took the slot or no
  URL was cited, the leftover URL is dropped and the website appended once. No-op when no
  website is configured. Staff Telegram/Zalo TEXT handles are never touched. Invoked once
  at the shared screen/repair convergence point in `cmd/scraper/outbound_actions.go`,
  covering BOTH the normal and reasoning=live paths. `contact_grounding.go` 188 (<200);
  tests split into `ensure_website_test.go` to keep each file <200.
- **Remaining risk:** the final output still depends on a company website being configured
  (`WebsiteURL` set) — when unset, no URL is cited (by design, never invented). CPD watch:
  the two prompt templates share more helper calls now (verify duplication stays ≤ 3%).

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

---
doc_type: architecture
status: accepted
owner: platform
last_reviewed: 2026-07-06
related_pr_or_issue: docs/reel-studio-platform-architecture (PR-R0)
---

# ADR: Reel Studio as a first-class platform module (v1)

> Part of the [architecture docs index](../INDEX.md). Companion of
> `DATABASE_OWNERSHIP.md` (data-plane doctrine) and `MODULE_BOUNDARIES.md`
> (import rules this module must follow).

## Status

Accepted for the PR-R0 foundation decision only. PR-R1 onward each need their
own review; this doc is not a blanket pre-approval of later PRs.

## Context

A prototype ("Reel Studio": AI-scripted short video → external render →
publish to Facebook) exists as **PR #229** (`origin/feat/reel-studio`,
commit `63c62f9b`, branched from `56b3a985`). It was never merged and its
base predates the database-boundary sprint (PR4–PR7, #225–#228) that landed
the modular PostgreSQL platform baseline (`internal/store/migrations/platform/`,
0100–0110), real apply validation, and the migration advisory lock.

**PR #229 is reference-only.** It is not merged, rebased, or conflict-resolved
into `main`. This ADR defines the v1 shape from current `main`; reusable logic
is ported by hand where it still fits, everything else is rewritten.

### Audit of PR #229 (55 files, +3997/-17)

| Layer | Files | Verdict |
|---|---|---|
| DB/migrations/store | `internal/store/migrations/0023_reel_tables__sqlite.up.sql`, `internal/store/reel/*` (store.go, reels.go, scripts.go, shots.go, reel_test.go) | **Discard the migration** (SQLite business schema — violates the data-plane doctrine below). Store package shape (accessor + per-concern files) is reusable. |
| RED-zone (outbound/ledger) | `internal/store/coordination/action_ledger.go` (+`post_reel`→`post` mapping), `internal/store/outbound/queue.go` (+`media_path`/`media_type` columns) | **Discard as bundled.** Touches the append-only ledger and the outbound spine in the same commit as the new feature — exactly what `feedback_append_only_correction_events` / staged-evolution doctrine forbids. Re-derive narrowly in PR-R6 if still needed. |
| service/workflow | `internal/services/reel/{ports,script,workflow,workflow_helpers,workflow_render,media,ffmpeg,httpkit,webhook_post,doc}.go` | Port design (`VideoRenderer`) and workflow shape are reusable reference; re-implement against the current store accessor pattern. |
| provider adapters | `provider_fal.go`, `provider_fpt.go`, `provider_heygen.go`, `render_real.go`, `render_cloudflare.go`, `render_fake.go` | Defer entirely (PR-R5). `render_fake.go` is the one adapter PR-R2 actually needs; review it then, don't port blind. |
| server/routes/webhook | `internal/server/reels/{handlers,routes,media,webhook,webhook_test,flow_test}.go`, `internal/server/reel_grounding.go`, `router.go`/`server.go` wiring | HMAC webhook pattern (`validHMAC`, `X-Reel-Signature`) is sound and reusable as-is in PR-R3. |
| config/env | `internal/config/config.go` (+43 lines: provider keys, `ReelWebhookSecret`, `RenderProvider`) | Reusable shape; add incrementally per PR (don't pre-declare PR-R5 provider keys in PR-R1). |
| frontend | `frontend/src/modules/autoflow/components/reels/*`, `views/ReelView.tsx`, `services/reelsService.ts`, `i18n/reelStrings.ts` | Defer entirely to PR-R4, behind a feature flag, default hidden. |
| docs/specs | `specs/REEL_FRONTEND_UX.md`, `docs/architecture/MODULE_BOUNDARIES.md`/`MODULE_OWNERSHIP.yml` additions | UX spec is reusable input for PR-R4. The module-boundary/ownership entries are reusable *content*, filed for real once code lands (PR-R1), not in this doc-only PR. |

## Decision

### 1. Product boundary

`AI script → human approval → render (external provider, per-shot) → final
assembled media → optional publish.` Publish is a distinct, later boundary
(§7) — the render pipeline does not imply auto-publish.

### 2. Data-plane ownership

Per `DATABASE_OWNERSHIP.md` "Data planes" (binding, 2026-07-05):

- **PostgreSQL platform plane** is the source of truth for all reel business
  state (reels, scripts, shots, render jobs, cost/spend). This is a reversal
  of PR #229, which put this schema in SQLite because the platform baseline
  didn't exist yet.
- **Object storage** (R2/Cloudflare or equivalent) holds media binaries
  (rendered clips, assembled final video). Postgres stores keys/URLs, never
  blobs.
- **SQLite** is not used for reel business state. It may hold purely local
  runtime scratch (e.g. a temp ffmpeg working dir path) if a later PR
  genuinely needs local-process state — that would be compatibility/cache,
  never source of truth, and must be called out explicitly when added.
- **No RAG/vector data.** Script grounding reads existing `ai`/knowledge
  accessors; it does not create new retrieval-plane storage.

### 3. Proposed platform tables (PR-R1 scope, not created by this PR)

New migration file(s) under `internal/store/migrations/platform/`, starting
at **0111** (confirmed free against `main` and every open PG-migration branch
as of 2026-07-06). `migrator_topology_test.go` and the existing platform files
(0102–0110 average ~100 lines) suggest one file is workable but PR-R1 should
split into `0111_platform_reel_core__postgres.up.sql` (reels, reel_scripts)
and `0112_platform_reel_render__postgres.up.sql` (reel_shots,
reel_render_jobs) if it runs long — size it when writing, don't force one file.

- `reels` — one row per video task. `org_id`, `status`, brief input, final
  output key, accrued cost.
- `reel_scripts` — versioned dialogue/shot-list/caption, approval flag.
- `reel_shots` — per-shot render state (planned/rendering/done/failed),
  provider + provider_job_id, cost, lease/attempts for orphan detection.
- `reel_render_jobs` (or fold into `reel_shots` if PR-R1 finds no need for a
  separate table — decide at implementation time, not here) — the
  idempotency/claim record guarding against duplicate paid-render submission
  on retry/crash.

All tables: `org_id BIGINT NOT NULL DEFAULT 0` + org-scoped index, per the
existing platform convention (see `0102_platform_leads`), and registered with
`scripts/check_tenant_isolation.sh`.

**Money invariant carried over from PR #229 (still correct):** once a shot's
render is started, spend is committed and cannot be cancelled — no
`cancelled` state reachable from `rendering`; a stuck/orphaned job surfaces
for a human, it is never auto-retried into a second charge. The idempotency
guard is **scoped to reel's own table**, not a reuse of the outbound spine's
CAS/lease mechanism (mixing those would itself be a boundary violation).

### 4. Store/module ownership

- `internal/store/reel` — new domain store package, following the existing
  accessor pattern (`store.Reel() *reel.Store`, alongside `store.Leads()`,
  `store.Crawl()`). Owns all reads/writes to the tables in §3.
- `internal/services/reel` — orchestration only (script → render → assemble
  workflow, spend gate, the `VideoRenderer` port). No raw SQL; goes through
  `store.Reel()` and, later, `store.Outbound()` for publish.
- `internal/server/reels` — HTTP handlers + webhook, thin transport layer
  only, per `MODULE_BOUNDARIES.md` layering rules.
- Module boundary/ownership entries (`MODULE_BOUNDARIES.md`,
  `MODULE_OWNERSHIP.yml`) are filed in PR-R1 when the package actually exists,
  not speculatively in this doc-only PR.

### 5. Provider boundary

`VideoRenderer` is a consumer-owned port in `internal/services/reel` (mirrors
the existing adapter-boundary pattern used for Facebook automation):

```go
type VideoRenderer interface {
    StartRender(ctx context.Context, req RenderRequest) (RenderHandle, error)
    Name() string
}
```

- PR-R2 ships a `fake` adapter only (zero-cost, deterministic, dev/CI default).
- PR-R5 adds real adapters (FAL text-to-video, HeyGen avatar, FPT.AI TTS,
  local ffmpeg stitch) — each gated by its own env var, absent/empty = adapter
  disabled. No real provider code lands before PR-R5.

### 6. Webhook / security

- Render-completion is reported async via a webhook, HMAC-SHA256 signed
  (`X-Reel-Signature` header over the raw body), same shape as PR #229's
  `validHMAC` — reusable as written.
- No unauthenticated state transition: a missing/invalid signature is
  rejected (401) whenever a secret is configured; the handler must also
  validate the callback's `org_id`/reel ownership before applying any state
  change (PR #229 did not do this explicitly — call out as a fix, not a
  straight port).
- Provider credentials (`FAL_KEY`, `HEYGEN_API_KEY`, `FPT_TTS_KEY`,
  `REEL_WEBHOOK_HMAC_SECRET`, …) are env-only, never committed, and the
  feature is inert with all of them unset.

### 7. Outbound boundary (RED zone — isolated)

Publishing a finished reel as a Facebook post (`post_reel` action) touches
`internal/store/coordination/action_ledger.go` and
`internal/store/outbound/queue.go` — both RED zone. This is **PR-R6**, a
separate, small, reviewed PR, landing after the render pipeline is proven.
**No action_ledger/outbound_messages changes ship in PR-R1 through PR-R5.**

### 8. Frontend boundary

The Reel tab/UI (PR-R4) ships behind a feature flag, default hidden, until
the backend contract (schema + service + API) is stable across PR-R1–R3.
`specs/REEL_FRONTEND_UX.md` from PR #229 is a reusable UX reference for that
PR, not a build-now spec.

### 9. Migration strategy

New PostgreSQL platform migration(s) numbered 0111 (+0112 if split), next
after the current 0100–0110 baseline. No SQLite reel migration. No changes to
existing tables outside the new reel-owned ones in PR-R1–R5 (the
`outbound_messages` column addition, if still needed, is scoped inside PR-R6
alongside the ledger change it pairs with).

### 10. PR train

| PR | Scope | Zone |
|---|---|---|
| R0 | This doc. | docs |
| R1 | Schema (0111/0112) + `internal/store/reel` store package + characterization tests. | GREEN/YELLOW (new package, no cross-boundary writes) |
| R2 | `internal/services/reel` workflow + fake `VideoRenderer` only. | YELLOW |
| R3 | `internal/server/reels` API + webhook skeleton (HMAC, org-scoped), wired but reachable only via direct API call (no frontend yet). | YELLOW |
| R4 | Frontend Reel tab, feature-flagged, default hidden. | GREEN (additive UI) |
| R5 | Real provider adapters (FAL/HeyGen/FPT + ffmpeg stitch), disabled unless configured. | YELLOW (external I/O, no shared-state risk) |
| R6 | `post_reel` outbound publish: action_ledger target-type mapping + `outbound_messages` media columns + policy row. | **RED** — own PR, own review, per `ESCALATION_PLAYBOOK.md` if any semantics are ambiguous. |

Each PR is independently reviewable and revertable; R1–R5 ship with the
feature functionally inert to end users until R4's flag is flipped.

## What is explicitly NOT ported from PR #229

- The SQLite migration file (`0023_reel_tables__sqlite.up.sql`) — schema is
  re-authored as a PostgreSQL platform migration.
- The `action_ledger.go` / `outbound/queue.go` edits — re-derived narrowly in
  PR-R6 only, never bundled with schema/service work.
- Any provider adapter code, pending its own review in PR-R5.
- The frontend components, pending PR-R4 and a feature-flag gate.
- The `MODULE_BOUNDARIES.md`/`MODULE_OWNERSHIP.yml` entries — filed for real
  in PR-R1 once `internal/services/reel` exists, not speculatively here.

## Rollback plan

- PR-R0 (this doc): revert the single commit; no code or schema exists yet.
- PR-R1 (schema): migrations are additive (`CREATE TABLE IF NOT EXISTS`) and
  own-table-only; rollback is a manual forward migration dropping the three
  (or four) new tables — safe before the feature is used, since nothing
  outside `internal/store/reel` reads them yet.
- PR-R2–R5: each adds an isolated package or an env-gated adapter; disabling
  is unsetting the relevant env var(s) or reverting the PR's commit, with no
  effect on any existing domain.
- PR-R6 (RED): rollback requires the same care as any action_ledger/outbound
  change — revert the commit and the `post_reel` policy row; no in-flight
  `post_reel` messages should exist yet at that point given R1–R5 gate real
  usage behind R4's flag.

## Next PR-R1 scope

`internal/store/reel` package: `reels`, `reel_scripts`, `reel_shots` (+
`reel_render_jobs` if kept separate) migrations at 0111(/0112), Go store
accessor (`store.Reel()`), org-scoped CRUD, and characterization tests —
no service, no HTTP, no provider code. Validated by
`scripts/check_tenant_isolation.sh`, `migrator_topology_test.go`, and
`scripts/ai_validate.sh`.

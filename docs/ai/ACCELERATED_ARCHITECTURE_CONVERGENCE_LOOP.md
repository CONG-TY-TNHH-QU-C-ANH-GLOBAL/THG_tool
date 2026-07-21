---
doc_type: ai
status: active
owner: platform
last_reviewed: 2026-07-01
related_pr_or_issue: docs/accelerated-architecture-convergence-loop
---

# Accelerated Architecture Convergence Loop

**Status:** OFFICIAL PROTOCOL (process). Invoked by `/thg-converge`.

Purpose: close architecture debt **faster** by running coherent, larger
convergence batches instead of tiny one-off prompts — while every existing
safety guard in `CLAUDE.md` still holds. This is a *mode* over the existing
authorities, not a new rulebook:

- Engineering guardrails (file size, SOLID, component structure, no god files)
  → `CLAUDE.md` ("Engineering Guardrails").
- Lane / feasibility / boundary vocabulary → `docs/architecture/BOUNDARY_MIGRATION_PLAYBOOK.md`.
- Hard cases / decision records → [`ESCALATION_PLAYBOOK.md`](ESCALATION_PLAYBOOK.md).
- Truth ownership / domain boundaries → `specs/domains/platform-foundation/features/runtime-topology/technical.md`,
  `internal/store/DOMAINS.md`.
- Report shape → [`AGENT_REPORT_TEMPLATE.md`](AGENT_REPORT_TEMPLATE.md) + §8 below.
- Validation → `scripts/ai_preflight.sh`, `scripts/ai_validate.sh`,
  `scripts/go_cognitive_check.sh`, `scripts/check_file_size.py`,
  `scripts/check_topology.sh`, `scripts/check_component_structure.py`.

Where this protocol and an authority above disagree, the authority wins. This
doc only changes *how much* Claude batches and how independently it decides
inside already-safe lanes — never what counts as safe.

Relationship to `docs/ai/ARCHITECT_SPRINT_MODE.md`: Architect Sprint Mode is
queue-item-driven (`docs/ai/queue/items/**`) and mission-scoped. This loop is
**survey-driven** — it does not require a pre-existing queue item, it scans
the live tree each run and picks the batch itself. Use whichever entrypoint
the user invokes; both defer to the same guardrails and never conflict on a
RED zone.

## Operating goal

Move living production code toward the agreed modular-monolith architecture:

- thin composition roots;
- bounded domain subpackages;
- existing store subpackages consumed via accessors (e.g. `Store.Outbound()`),
  not the legacy flat `*Store` surface;
- fewer god packages/files;
- fewer deprecated shims/wrappers;
- no speculative abstractions;
- no audit-only PRs unless no safe batch exists (see §3).

## 1. Sync

- Fetch and start from latest `origin/main`.
- Check current branch cleanliness (`git status --short`); if dirty with
  unrelated work, stop and ask rather than stash/discard it.
- Never `git add -A`. Stage explicit paths only.
- Never stage: queue-reconcile `.md` files (`docs/ai/queue/items/**` outside
  the item this batch itself touches), soak-test artifacts
  (`specs/RETRIEVAL_SOAK_REPORT.md`), env files, `.mcp.json`.
- Run `scripts/ai_preflight.sh` (import-boundary, file-size, docs-governance,
  autopilot-queue guards, read-only).

## 2. Survey

- Scan current architecture hotspots: `specs/domains/platform-foundation/decisions/component-hotspots.md`,
  `scripts/check_component_structure.py` output, `scripts/check_file_size.py`
  allowlist (`scripts/file_size_allowlist.txt` — legacy files above the
  200-line limit are the natural extraction backlog), remaining deprecated
  wrapper/alias files (grep for `Deprecated:` / `_aliases.go` / shim
  comments), and `*AppStore`-style god objects.
- Identify the **top 3 convergence candidates**.
- Score each by: leverage (how much debt it retires), risk (GREEN/YELLOW/RED
  per `BOUNDARY_MIGRATION_PLAYBOOK.md`), PR size, Sonar risk (S3776/S107/
  duplication exposure), and behavior risk (does it touch a controlled zone).
- Prefer actual move / extraction / deletion / migration work over
  audit-only output — an audit PR is only acceptable when survey finds no
  safe batch (see stop conditions in §5 / §9).

## 3. Select

- Choose the **highest-leverage safe batch** from the top 3.
- Batch size should be **larger than the old one-file-at-a-time cadence** —
  avoid over-fragmenting into tiny PRs.
- Prefer one coherent boundary per PR (one subpackage, one wrapper family, one
  caller-migration sweep).
- Combine low-risk and medium-risk mechanical caller migrations when they
  share the same goal (e.g. "migrate every remaining caller off alias X and
  delete X" is one PR, not N).
- Keep RED/hot-path semantic cutovers staged (characterize, don't cut over)
  but do not over-split harmless mechanical changes just to keep PRs tiny.

## 4. Implement

- Execute the selected batch without asking approval for every small file —
  permission is required only for the stop-and-ask conditions in §5.
- Use existing architecture patterns already established in the repo
  (subpackage + accessor, domain contract, projection) — do not invent new
  ones.
- No new repository interfaces/ports in the store layer unless the existing
  repo rules (`internal/store/DOMAINS.md`) explicitly allow it.
- Consumer-owned ports only where they unlock a real service/package move —
  never speculative.
- No speculative abstraction, no broad unrelated cleanup, no noisy
  formatting/import churn outside touched lines (Ponytail discipline:
  `ponytail:ponytail` / `ponytail:ponytail-review` when available).

## 5. Guardrails (never weaken these)

- Preserve route paths / auth / ordering.
- Preserve request/response wire shape.
- Preserve tenant/org filtering (every tenant feature keeps `org_id`
  ownership checks — `CLAUDE.md` "Hard Rules").
- Preserve queue/CAS/lease/outbox/`action_ledger` semantics
  (`specs/domains/facebook-sales-intelligence/features/outbound-actions/implementation/append-only-ledger.md`).
- Preserve auth/security/session behavior.
- No schema/migration changes unless explicitly selected as a controlled
  migration PR (its own item, its own decision record).
- Keep `installOutboundHooks` and equivalent coordination hooks untouched
  unless the PR explicitly targets them.
- No Sonar suppressions. No Sonar config changes.

## 6. Acceleration rule

- If a changed file trips the cognitive-complexity or file-size guard, fix it
  **in the same PR** with a minimal, behavior-preserving helper extraction —
  do not abandon an otherwise-good batch because one touched file needs a
  small extraction.
- Continue iterating until local validation is green; do not split a coherent
  batch across PRs just to dodge a guard failure that a small extraction
  would fix.

## 7. Validation loop

Run, in order, fixing failures before reporting:

1. Targeted `go test` for touched packages.
2. `go test ./...`.
3. `scripts/ai_validate.sh` (wraps `go build`/`go vet`/`go test`, import-
   boundary, file-size, cognitive-complexity, docs-governance, autopilot-queue
   guards, and `git diff --check`).
4. `scripts/check_topology.sh` when a store/domain boundary was touched.
5. `scripts/check_component_structure.py` when files were added/moved
   (warn-only, but read the output).
6. `git diff --check` (already covered by `ai_validate.sh`, run standalone if
   validating incrementally).
7. If Sonar later reports New Code issues on the pushed branch, fix them in a
   follow-up commit on the same branch — same behavior, no suppression, no
   config change.

## 8. Report

Every convergence PR ends with:

```text
Accelerated Convergence:
- selected boundary and why:
- candidates considered (top 3) and scores:
- files moved / changed / deleted:
- wrappers/shims migrated or deleted (if applicable):
- behavior-preservation proof:
- risk statement:          (route/wire/auth/tenant/CAS/ledger/schema — touched? preserved how?)
- tests / guards run:      (§7, with pass/fail)
- Sonar expectation:       (New Code = 0, S3776/duplication notes)
- rollback plan:
- next recommended batch:
```

## 9. Stop-and-ask conditions

Stop and produce an `ESCALATION_PLAYBOOK.md`-style decision record (do not
force a workaround) when the batch would require:

- a product-visible behavior change;
- an auth/security/session semantics change;
- a schema/migration design decision;
- a queue/CAS/lease/outbox/`action_ledger` semantic cutover;
- a public DTO/wire-shape change;
- unclear tenant-isolation behavior;
- deleting code whose runtime usage is uncertain (proof-based deletion only —
  see `[[feedback_deletion_lane_discipline]]`: reachability ≠ delete);
- changing CI/Sonar policy or config;
- an import cycle that forces a package-ownership decision.

A precise, stopped report with A/B/C options and a recommended default is a
successful outcome — same standard as `ARCHITECT_SPRINT_MODE.md` §3 RED/BLOCKED.

## 10. Current default backlog

Living note, refresh after each converge PR — this is guidance for picking
the next batch, not a status board (per-PR state belongs in the PR itself,
not here):

1. Continue Candidate B: retire remaining deprecated wrappers in
   `internal/store/outbound_aliases.go` in accelerated batches.
2. Next PR migrates all remaining non-hot-path outbound alias callers in one
   coherent PR and deletes wrappers that become unused as a result.
3. The final outbound-alias PR handles the hot-path CAS/claim/finalize/lease
   wrappers, with characterization tests proving behavior is unchanged before
   the wrappers are removed (RED zone — safe migration phase only, see
   `ARCHITECT_SPRINT_MODE.md` §4.2).
4. After Candidate B is fully retired, survey `*AppStore` dissolution as a
   staged multi-PR project (new item, do not fold into the alias PRs).
5. Do not return to agent decomposition (`internal/agent`) unless a new
   blocker appears — it is sufficiently converged.
6. Do not open crawl-boundary PRs — the crawl/crawler survey found that
   boundary already converged.

## 11. Push policy

Push only after `scripts/ai_validate.sh` passes. One coherent branch per
convergence PR. **Never merge.**

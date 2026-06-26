# AUTOPILOT_QUEUE

This file is the stable queue index and operating policy.

Do not use this file as a mutable PR status board.
Per-item lifecycle state lives in `docs/ai/queue/items/*.md`.

## Lockless queue rule

Feature/work PRs must not edit this central queue file unless the task is explicitly queue-governance work.

A work PR may update only its own queue item file under `docs/ai/queue/items/`.

This prevents merge conflicts when multiple branches are open.

## Queue item files

Each queue item must live under:

`docs/ai/queue/items/<ID>-<slug>.md`

Each item file must contain:
- id
- status
- lane
- risk
- depends_on
- parallel_safe
- branch
- pr_url
- goal
- scope
- constraints
- validation
- result / notes

## Status lifecycle

- READY: available to execute.
- IN_PROGRESS: Claude is currently working on it.
- REVIEW: branch pushed, waiting for human PR review / CI / Sonar / merge.
- DONE: merged into main.
- BLOCKED: cannot proceed without human/product/credential/destructive decision.

Claude may update an item file when:
- starting work: READY -> IN_PROGRESS
- pushing branch: IN_PROGRESS -> REVIEW
- user reports merge success: REVIEW -> DONE
- stop condition requires human decision: IN_PROGRESS -> BLOCKED

Claude must not mark DONE unless the user explicitly says the PR was merged into main.

## Dependency rule

YELLOW and RED items are sequential by default.

Claude must not start an item if any `depends_on` item is not DONE, unless the user explicitly authorizes parallel work and the item is GREEN + parallel_safe.

Dependency states (enforced by `scripts/ai_queue_check.sh`):

- **Missing dependency id** (references an item that does not exist) = invalid queue → the check FAILS.
- **Existing dependency not yet DONE** (READY/IN_PROGRESS/REVIEW/BLOCKED) = normal *waiting* state → the dependent item is not executable, but this is NOT a failure.

The first *executable* READY item is the first READY item whose dependencies are all DONE; if every READY item is waiting on a non-DONE dependency, there is simply no executable item right now (not an error).

## Sprint mode

User may say:

`Autopilot: run green sprint, max N PRs.`

Rules:
- execute at most N GREEN items,
- only items with `parallel_safe: true`,
- no unmet dependencies,
- disjoint scopes/files,
- one PR per branch,
- each PR updates only its own item file,
- push each branch after validation,
- never merge,
- stop immediately if any item becomes YELLOW/RED/hard Sonar/ambiguous.

## Queue index

- PR31D: `docs/ai/queue/items/PR31D-facebook-crawl-session-fake-seam.md`
- PR31E: `docs/ai/queue/items/PR31E-facebook-crawl-readiness-runtime-edge-coverage.md`
- PR32A: `docs/ai/queue/items/PR32A-facebook-operator-ux-status-flow.md`

### Self-Feeding Architecture Epic (generated 2026-06-26)

Sequenced decomposition queue from a topology scan of `internal/workspace_knowledge`,
`internal/store`, `internal/server`, `internal/drivers/copilot`, `cmd/scraper`.
Lanes: GREEN = package-internal pure/file-responsibility cleanup (no import-boundary,
no DB/auth/ledger/connector/queue/runtime semantics); YELLOW = behavior-preserving
move-only that crosses an import boundary (sequential, by deps); RED = audit-only,
`status: BLOCKED`, human decision required (no autonomous implementation). In Go a
folder move is a package/import-boundary change, so YELLOW items merge sequentially.

GREEN (executable):
- ARCHWK1 (IN_PROGRESS — this PR): `ARCHWK1-governance-output-validation-split.md`
- ARCHWK2: `ARCHWK2-products-canonical-split.md`
- ARCHWK3: `ARCHWK3-retrieval-helpers-rename.md`
- ARCHST1: `ARCHST1-store-test-fallback-migration.md`
- ARCHSV1: `ARCHSV1-crawl-direct-post-helper-extract.md`
- ARCHCP1: `ARCHCP1-agent-brain-split.md`
- ARCHCP2: `ARCHCP2-agent-preflight-split.md`
- ARCHCM1: `ARCHCM1-action-args-split.md`

YELLOW (move-only, sequential by deps):
- ARCHWK4: `ARCHWK4-soak-internal-grouping.md`
- ARCHSV2 → ARCHSV3, ARCHSV4: agent finalize / crawl_ingest / outbox subpackages
- ARCHCP3 (needs CP1+CP2) → ARCHCP4: copilot intent / agent subpackages
- ARCHCM2 (needs CM1+CM-R1) → ARCHCM3 (needs CM2+ST-R3) → ARCHCM4 (needs CM-R1+CM-R2)

RED (audit-only, BLOCKED — human decision):
- ARCHST-R1 append-only ledger; ARCHST-R2 connector lease/CAS; ARCHST-R3 direct-post boundary
- ARCHSV-R1 workspace browser-orchestration
- ARCHCM-R1 account-scope RBAC consolidation; ARCHCM-R2 crawl runtime semantics

### Backlog (not yet item files)

- Sonar Ponytail cleanup batch (GREEN) — fix low-risk Sonar New Code issues only when explicitly requested.
- Docs taxonomy migration (GREEN/YELLOW) — gradually move legacy root/spec/debt docs into the governed taxonomy (git mv, update references).

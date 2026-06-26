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

23 sequenced decomposition items (ARCH*) from a topology scan of
`internal/workspace_knowledge`, `internal/store`, `internal/server`,
`internal/drivers/copilot`, `cmd/scraper`. The human-readable map — grouped by
component then lane, with active/next/blocked items, dependency chains, and a
Mermaid diagram — lives in **[`docs/ai/queue/INDEX.md`](queue/INDEX.md)**. The
item files under `docs/ai/queue/items/ARCH*.md` remain the source of truth.

### Backlog (not yet item files)

- Sonar Ponytail cleanup batch (GREEN) — fix low-risk Sonar New Code issues only when explicitly requested.
- Docs taxonomy migration (GREEN/YELLOW) — gradually move legacy root/spec/debt docs into the governed taxonomy (git mv, update references).

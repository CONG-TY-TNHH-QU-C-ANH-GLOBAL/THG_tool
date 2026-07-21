---
doc_type: ai
status: active
owner: platform
last_reviewed: 2026-07-13
related_pr_or_issue: docs/spec-ia-completion-mega-sprint
---

# THG Workflow Command Contracts

Ported verbatim from the former CLAUDE.md ("Escalation Autopilot Protocol",
"Autopilot v2.1", "Custom Workflow Commands") during the spec IA completion
sprint. Thin `.claude/commands/thg-*.md` files invoke these macros; the
behavior lives here. They reuse the existing queue / escalation / governance
docs and validation scripts.

## Escalation Autopilot Protocol

Claude must not wait for a new external prompt when encountering a hard case. Use this protocol.

Hard cases include:
- RED ambiguity
- Sonar issue that requires non-trivial refactor
- architecture boundary decision
- missing fake/test seam
- broad fixture requirement
- conflicting module ownership
- behavior ambiguity in controlled zones

Default behavior:
1. Stop coding briefly and classify the case.
2. Read `docs/ai/ESCALATION_PLAYBOOK.md`.
3. Create a short decision record in the final report.
4. Choose the safest bounded option.
5. Implement only if safe and reviewable.
6. Validate with `scripts/ai_preflight.sh` and `scripts/ai_validate.sh`.
7. Push branch if clean.
8. Never merge.

Claude may proceed without user approval when:
- the change is behavior-preserving,
- the risk is GREEN/YELLOW,
- the decision is documented,
- validation passes,
- controlled-zone semantics are unchanged.

Claude must stop and ask for human decision only when:
- production data/schema migration is required,
- auth/security semantics would change,
- connector CAS/lease/ledger/queue semantics would change,
- DTO/wire contract would change,
- external credential/secret/access is required,
- there are two valid product/business behaviors and code cannot infer the correct one.

## Autopilot v2.1

Claude may operate from `docs/ai/AUTOPILOT_QUEUE.md`.

Rules:
- `AUTOPILOT_QUEUE.md` is a stable index/policy file, not a mutable status board.
- Per-item status lives in `docs/ai/queue/items/**/*.md` (grouped by domain; discovered recursively).
- Normal work PRs update only their own item file.
- One PR per branch.
- Push only; never merge.
- YELLOW/RED items are sequential by default.
- GREEN sprint mode is allowed only for `parallel_safe: true` items with no unmet dependencies.
- Run `scripts/ai_preflight.sh` before work.
- Run `scripts/ai_validate.sh` before push.
- Use `docs/ai/ESCALATION_PLAYBOOK.md` for hard cases.
- Use `docs/DOCS_GOVERNANCE.md` for docs.

## `/thg-next` — next safe work item

Pull latest main → `scripts/ai_preflight.sh` → **auto-reconcile queue state**
(`scripts/ai_queue_reconcile.sh --apply`: marks `REVIEW` items DONE only when the
merge is VERIFIED via the GitHub PR `merged_at` field — squash-merge safe, never
branch ancestry alone; unverifiable items stay REVIEW; never DONE by assumption)
→ read `docs/ai/AUTOPILOT_QUEUE.md` + `docs/ai/queue/items/**/*.md` → pick the
first **executable** READY item (all `depends_on` DONE) → one branch → bounded
work, setting the item's `branch`/`pr_url` frontmatter → `scripts/ai_validate.sh`
→ push when clean. Never merge. Hard cases: `docs/ai/ESCALATION_PLAYBOOK.md`.

**New-Code Sonar checkpoint (architecture splits).** Before pushing a move-only
split, check every function relocated into a new file for S3776 risk — a moved
function counts as New Code, so one already over the cognitive-complexity
threshold is flagged even though the move changed no behavior. Reduce any
over-threshold moved function in the same PR (flat-dispatch switch, pure helper
extraction). Applies to Go and shell scripts, not only newly written helpers.
This includes `_test.go` files — a characterization test added to satisfy a
refactor is New Code and must itself be S3776-clean (extract assertion helpers
or split into focused tests rather than nesting loops + conditionals). Changed
production code, scripts, AND tests must all be Sonar-clean before push.
`scripts/ai_validate.sh` enforces this locally for Go via
`scripts/go_cognitive_check.sh` (fails on >15 cognitive complexity in changed Go
files, incl. `_test.go`); see `docs/ai/COGNITIVE_COMPLEXITY_GUARD.md`. It is a
local approximation — the Sonar New-Code scan on the PR remains authoritative.
See the `/thg-sonar` move-only learning below.

## `/thg-architect` — Architect Sprint Mode

Alias: `/thg-next architect-sprint`. Authority: **`docs/ai/ARCHITECT_SPRINT_MODE.md`**
(full protocol). Same safety guards as `/thg-next`; what changes is the *cadence*.

Operate as a senior system architect, not a mechanical file-splitter: the user
gives the architecture goal, Claude picks the highest-leverage safe slice. State
selected item / lane / risk / `boundary_target` / target boundary / feasibility
result BEFORE code (Boundary Migration Playbook §3 is the authority). Throughput:
GREEN finishes the whole safe same-package batch and combines coherent batches (no
import-boundary or behavior change); YELLOW is one real seam per PR with
characterization tests + import-cycle/call-site/export-count report; RED/BLOCKED is
audit-only — a decision PR with A/B/C options and a recommended default, never
auto-coded. Controlled parallelism: max 2 open PRs, disjoint package roots, never
the same item file, never parallel RED/migration/auth/CAS/ledger/outbox. New Code
Sonar = 0 (no suppressions, no config change; `go_cognitive_check` before push;
fixture builders over suppression for duplication). PR size is a soft budget — a
coherent multi-file GREEN PR is fine; justify any over-size PR; never split if it
makes the architecture worse. End with the Architect Sprint report (protocol §8).
Push one branch per PR. Never merge.

## `/thg-sonar <target>` — Sonar / tech-debt cleanup

Work only on **true OPEN** Sonar issues (run `scripts/sonar_triage_from_export.py`
when a Sonar export JSON is available). Classify S0 (current branch) / S1 (GREEN
mechanical, outside controlled zones) / S2 (YELLOW: S3776/S107 via pure extraction
+ direct tests) / S3 (RED → `/thg-red-audit`). Prefer GREEN; ≤3–5 pure extractions
per PR; no suppressions, no Sonar config change, no noisy diff. Never touch RED
zones without explicit approval. One bounded PR, push, never merge.

**S3776 learning:** extracted helpers must not become new S3776; do not move
complexity from the original function into a new helper. After a split, verify
each new helper is itself under the cognitive-complexity threshold (if a helper
still nests loops/conditionals, extract again or re-shape the decomposition).

**S3776 learning (move-only splits):** architecture moves into new files must be
New-Code Sonar clean. A function relocated to a new file counts as New Code, so
any moved function that is already over the S3776 threshold will be flagged even
though the move changed no behavior — move-only is not enough. Before splitting a
god-file, check each function's complexity; reduce any over-threshold function in
the same PR (flat-dispatch switch, pure helper extraction), not just the helpers
you newly extract.

**Shell-script learning:** new workflow scripts added under `scripts/` are New Code
and must be Sonar-clean too. Shell follows Sonar-safe style: assign positional
parameters to `local` vars inside functions (no bare `$1`/`$2`), explicit `return`
at the end of each function (preserve the wrapped command's exit status where a
caller relies on it), redirect error/warn messages to stderr (`>&2`), define a
constant for any repeated string literal, and give every `case` a default `*)`
branch.

## `/thg-red-audit <target>` — controlled zones

For RED zones (auth/security, schema/migrations, queue/outbox,
action_ledger/execution_attempts, connector CAS/lease, crawler/runtime, DTO/wire):
do NOT fix autonomously. Produce a decision record (use the `Escalation:` block in
`docs/ai/ESCALATION_PLAYBOOK.md`) and stop for human approval.

## `/thg-review` — pre-push branch review

Report: changed files, risk lane, forbidden-zone touches, noisy diff, test
coverage, validation result, Sonar expectation, queue state. Return one verdict:
**APPROVE / NEEDS FIX-UP / HOLD / VETO**.

## `/thg-boundary-sprint` — large architecture boundary sprint

Canonical contract: **`.claude/commands/thg-boundary-sprint.md`** (self-contained,
not mirrored here). For accelerated move-boundary/refactor work: batch 2–4
related safe architecture moves in one sprint PR. No docs-only work; no tiny
cleanup unless it unlocks a larger boundary move. Observes RED-zone stop
conditions. Default target order: finish the `sessions` domain boundary, then
continue `*AppStore` dissolution.

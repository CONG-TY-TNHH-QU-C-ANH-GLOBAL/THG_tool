Execute the THG **Architect Sprint Mode** workflow.

Authority: `docs/ai/ARCHITECT_SPRINT_MODE.md` (the protocol). `/thg-next architect-sprint`
is an alias for this command.

Operate as a senior system architect / principal engineer under **Principal Engineer
Autonomy v2** (protocol §4): optimize for safe forward motion; self-select the
highest-leverage **code** slice; do not fall back to docs while a code slice exists;
touch RED zones only as a safe migration phase; keep behavior-changing cutover
controlled. Run as **review-bracketed implementation** (protocol §4a): senior + Ponytail
passes before coding, senior review pass before push.

Use:
- `docs/ai/ARCHITECT_SPRINT_MODE.md` (autonomy v2 §4 · skill passes §4a · report §8)
- `docs/architecture/BOUNDARY_MIGRATION_PLAYBOOK.md` (lane + feasibility authority)
- `docs/ai/AUTOPILOT_QUEUE.md` + `docs/ai/queue/items/**/*.md` (queue + per-item state)
- `docs/ai/ESCALATION_PLAYBOOK.md` (RED cutover/ambiguous → decision record, A/B/C)
- `scripts/ai_preflight.sh`, `scripts/ai_validate.sh`, `scripts/go_cognitive_check.sh`, `scripts/check_file_size.py`

Steps:
1. Sync `origin/main`; run `scripts/ai_preflight.sh`. **Non-Blocking Queue Reconcile
   (git hygiene):** then run `bash scripts/queue_reconcile_pr.sh --check` to detect stale
   merged queue items (`status: REVIEW` + merged PR) in READ-ONLY mode. If any are stale,
   run `bash scripts/queue_reconcile_pr.sh --push` — it isolates the metadata writes in a
   throwaway `git worktree`, commits ONLY `docs/ai/queue/items/**/*.md` onto a dedup'd
   `chore/queue-reconcile-<date>` branch, pushes it, prints the link, and NEVER merges (see
   `/thg-queue-reconcile`). This keeps the primary working tree clean and queue metadata
   OUT of your code PR. Then CONTINUE the sprint immediately — do NOT wait for the queue PR
   to merge. For dependency selection use the **effective queue state**: an item the
   reconcile proved merged may be treated as DONE, but its `.md` file MUST NOT be staged or
   committed in your code PR.
2. **Skill discovery (§4a.A):** inventory `.claude/skills/**/SKILL.md`,
   `.claude/commands/*.md`, the bundled skills list, and Agent subagent types
   (Ponytail, code-review, senior-architect, code-reviewer, senior-backend,
   senior-security, …). Report what is available. **Never claim a skill was invoked
   unless it exists and was invoked; otherwise emulate its checklist and say so.**
3. **Before coding:** run the **Ponytail / minimalism pass (§4a.B)** — invoke
   `ponytail:ponytail` if present, else emulate — and the **senior architecture pass
   (§4a.C)** (senior-architect / code-reviewer skill or Agent, else emulate). State
   selected item / lane / risk / boundary_target / feasibility / RED touch points /
   behavior-preservation invariants / migration pattern / rollback BEFORE touching code.
4. **Implement (§4a.D)** the self-selected highest-leverage code slice under Autonomy
   v2: GREEN finishes the whole safe batch; YELLOW is one seam + characterization tests
   + import-cycle/call-site/export report; RED touch only as a §4.2 safe migration
   phase (port/facade/adapter/characterization/shadow/additive/dual-write/flagged/
   expand-phase-1/move-after-topology-proof) — keep the old path authoritative. A
   behavior-changing cutover or §4.3 item → stop with a decision record.
5. **Before push:** run the **senior review pass (§4a.E)** — `ponytail:ponytail-review`
   + `/code-review` (or code-reviewer Agent), else emulate — for minimalism / boundaries
   / behavior / tests / Sonar. New Code Sonar = 0 (no suppressions/config change);
   `go_cognitive_check` before push.
6. Validate (`scripts/ai_validate.sh` + relevant guards + `git diff --check`), push one
   branch per PR, end with the expanded §8 Architect Sprint report.

Controlled parallelism: max 2 open PRs, disjoint roots, never the same item file,
never parallel RED/migration work. Push when clean. **Never merge.**

Code-PR staging hygiene (binding): a code PR may stage ONLY (a) the production/test files
the selected item needs, (b) the selected queue item's own `.md`, and (c) `.md` files of
direct child items the selected item creates. It must NEVER stage unrelated
`docs/ai/queue/items/**/*.md` — reconciled metadata for OTHER items rides only on the
`chore/queue-reconcile-*` branch from step 1, never on a code branch. **Never `git add -A`**
— stage explicit paths. If a stale queue `.md` is dirty in the working tree at push time,
that is a step-1 miss: restore it (`git checkout -- <path>`) and let the queue-reconcile
flow own it; do not commit it here.

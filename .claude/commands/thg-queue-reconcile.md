Reconcile merged architecture queue items into a dedicated, isolated PR — NON-BLOCKING
and with strict git hygiene. Queue metadata (`status: REVIEW → DONE`, `pr_url` backfill)
must NEVER pollute a code PR or dirty the primary working tree.

Authority: `docs/ai/ARCHITECT_SPRINT_MODE.md` §"Non-Blocking Queue Reconcile".

This is the one and only mechanism for writing reconciled queue metadata to a branch.
It does the whole flow for you:

```bash
bash scripts/queue_reconcile_pr.sh --push
```

`scripts/queue_reconcile_pr.sh --push` exactly:
1. Detects stale merged queue items (read-only, via `scripts/ai_queue_reconcile.sh` —
   GitHub `merged_at`-verified; NEVER marks an open/unmerged PR item DONE).
2. If none are stale → prints "queue is current" and exits (no branch, no push).
3. Otherwise applies the metadata updates inside a THROWAWAY `git worktree` checked out
   from `origin/main` — the primary working tree is never touched.
4. Stages ONLY `docs/ai/queue/items/**/*.md` (the worktree is a clean `origin/main`
   checkout, so only the reconcile's own writes are staged; it never runs `git add -A`).
5. Commits with message `chore(queue): reconcile merged architecture items`.
6. Pushes a dedup'd `chore/queue-reconcile-<date>` branch — **reusing an existing open
   reconcile branch** (force-with-lease) instead of opening a duplicate PR; if that
   branch already reflects the merged state it reports "nothing new to push".
7. Prints the PR/compare link, removes the worktree, and **never merges**.

Rules:
- Start from a clean `main` and pull `origin/main` first (the script bases the worktree on
  `origin/main`, so the reconcile reflects authoritative merged state regardless of which
  branch you are on).
- Do NOT touch production/source code in this flow.
- Do NOT merge the reconcile PR.
- If `git worktree` is unavailable, the script bails safely (primary tree untouched) — it
  NEVER falls back to `git reset --hard`/`restore` on the primary tree.
- Report the pushed branch + compare link (or the "already current" / "nothing stale"
  result).

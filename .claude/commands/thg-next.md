Execute the THG `/thg-next` workflow defined in `CLAUDE.md` (Custom Workflow Commands).

Use:
- `docs/ai/AUTOPILOT_QUEUE.md`
- `docs/ai/queue/items/**/*.md` (grouped by domain; discovered recursively)
- `docs/ai/ESCALATION_PLAYBOOK.md`
- `docs/DOCS_GOVERNANCE.md`
- `scripts/ai_preflight.sh`
- `scripts/ai_queue_reconcile.sh`
- `scripts/ai_validate.sh`

Steps:
1. Sync latest `origin/main`; run `scripts/ai_preflight.sh`.
2. **Auto-reconcile queue state BEFORE selecting:** run
   `bash scripts/ai_queue_reconcile.sh --apply`. It marks `status: REVIEW`
   items DONE only when their merge is VERIFIED via the GitHub PR `merged_at`
   field (squash-merge safe — never branch ancestry alone), and backfills
   `pr_url`. Items it cannot verify stay REVIEW and are reported — never mark
   DONE by assumption.
3. Pick the first executable READY item (all `depends_on` DONE; skip BLOCKED).
4. One branch, one bounded PR. Set the selected item's `branch` (and `pr_url`
   when known) in its frontmatter — the item file stays the source of truth.

Push when clean. Do not merge.

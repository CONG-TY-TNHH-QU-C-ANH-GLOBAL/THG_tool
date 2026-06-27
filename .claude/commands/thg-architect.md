Execute the THG **Architect Sprint Mode** workflow.

Authority: `docs/ai/ARCHITECT_SPRINT_MODE.md` (the protocol). `/thg-next architect-sprint`
is an alias for this command.

Operate as a senior system architect, not a mechanical file-splitter: pick the
highest-leverage safe slice, combine coherent GREEN batches, create enabling
seams, and finish a whole staged item when same-package and safe. Stop/re-scope
only when the boundary is genuinely unsafe. Keep New Code Sonar clean.

Use:
- `docs/ai/ARCHITECT_SPRINT_MODE.md` (mission/throughput/autonomy/report rules)
- `docs/architecture/BOUNDARY_MIGRATION_PLAYBOOK.md` (lane + feasibility authority)
- `docs/ai/AUTOPILOT_QUEUE.md` + `docs/ai/queue/items/**/*.md` (queue + per-item state)
- `docs/ai/ESCALATION_PLAYBOOK.md` (RED/ambiguous → decision record, A/B/C + default)
- `docs/ai/AGENT_REPORT_TEMPLATE.md` (base report shape)
- `scripts/ai_preflight.sh`, `scripts/ai_validate.sh`, `scripts/go_cognitive_check.sh`, `scripts/check_file_size.py`

Steps:
1. Sync latest `origin/main`; run `scripts/ai_preflight.sh`.
2. Pick the highest-leverage executable item(s) (READY, all `depends_on` DONE).
   State selected item / lane / risk / boundary_target / target boundary /
   feasibility-before-code result BEFORE touching code.
3. Execute one coherent slice under the throughput rules:
   - GREEN: finish the whole safe same-package batch; combine coherent batches;
     no import-boundary change; no behavior change.
   - YELLOW: one real seam; characterization tests + import-cycle/call-site/
     export-count report; behavior-preserving.
   - RED/BLOCKED: do not auto-code — produce an audit/decision PR with A/B/C and a
     recommended default.
4. Controlled parallelism: max 2 open PRs, disjoint package roots, never the same
   item file, never parallel RED/migration/auth/CAS/ledger/outbox work.
5. New Code Sonar = 0 (no suppressions, no config change); `go_cognitive_check`
   before push; prefer fixture builders over suppression for duplication.
6. Validate (`scripts/ai_validate.sh` + relevant guards), push one branch per PR,
   end with the Architect Sprint report (§8 of the protocol).

Push when clean. **Never merge.**

Execute the THG `/thg-review` workflow defined in `CLAUDE.md` (Custom Workflow Commands).

Review the current branch before push/merge and report:
- changed files
- risk lane (GREEN / YELLOW / RED)
- forbidden-zone touches (auth, schema, queue/outbox, ledger, connector, runtime, DTO/wire)
- noisy diff
- test coverage
- validation result (`scripts/ai_validate.sh`)
- Sonar expectation
- queue state (`scripts/ai_queue_check.sh`)

Return one verdict: APPROVE / NEEDS FIX-UP / HOLD / VETO. Do not merge.

Execute the THG `/thg-red-audit` workflow defined in `CLAUDE.md` (Custom Workflow Commands).

For RED / controlled zones: auth/security, schema/migrations, queue/outbox,
action_ledger/execution_attempts, connector CAS/lease, crawler/runtime, DTO/wire.

Do NOT fix autonomously. Produce a decision record using the `Escalation:` block in
`docs/ai/ESCALATION_PLAYBOOK.md` and stop for human approval. Do not merge.

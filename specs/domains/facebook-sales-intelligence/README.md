# Domain: facebook-sales-intelligence

Ownership domain (kind: **product**) — find fresh Facebook leads, qualify them
against business context, and convert them through approved engagement.

Structure: `experiences/` hold customer/operator contracts (business.md +
experience.md); `features/` hold the technical contracts and their
implementation / decisions / evidence / runbooks. A feature may support several
experiences; experiences reference features, never duplicate them.

## Experiences

- [fresh-lead-discovery](experiences/fresh-lead-discovery/README.md) — operator
  runs multi-group campaigns and receives provably-fresh leads with visible
  "why included/excluded".
- [lead-management](experiences/lead-management/README.md) — shared leads with
  owned execution, lifecycle tabs, typed archiving, no-spam channel discipline.
- [engagement-approval](experiences/engagement-approval/README.md) — outbound
  engagement is approval-governed by default, deduplicated, identity-verified,
  and ends in honest verified outcome states.

## Features

- [multi-group-fresh-lead-crawl](features/multi-group-fresh-lead-crawl/README.md)
  — campaign/queue/run orchestration with a freshness gate.
- [account-safety](features/account-safety/README.md) — account-runtime state
  machine, concurrency budgets, cooldown, safety boundaries.
- [lead-ingestion](features/lead-ingestion/README.md) — post→lead extraction
  and scoring behavior.
- [lead-lifecycle](features/lead-lifecycle/README.md) — lifecycle states, work
  queue, archiving.
- [multi-actor-coverage](features/multi-actor-coverage/README.md) — per-actor
  coverage and dedup policy across a workspace's accounts.
- [comment-automation](features/comment-automation/README.md) — comment
  execution, forensics, async reverify, human-verify fallback.
- [comment-intelligence](features/comment-intelligence/README.md) —
  knowledge-first comment decisioning, policy gate, actor verification (draft).
- [direct-post-intake](features/direct-post-intake/README.md) — durable
  operator-submitted post link → lead + queued comment workflow.
- [outbound-actions](features/outbound-actions/README.md) — tenant-isolated
  outbound state machine with transition ledger and policy-driven dedup.

## Not yet migrated

The rest of this domain's specs still live in the flat legacy folders
(`specs/facebook/` remainder and `specs/copilot/`) and are tracked in
[`specs/SPEC_REGISTRY.json`](../../SPEC_REGISTRY.json). They migrate here in
later batches.

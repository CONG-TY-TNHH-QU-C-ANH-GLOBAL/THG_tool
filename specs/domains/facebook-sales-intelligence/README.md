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

## Features

- [multi-group-fresh-lead-crawl](features/multi-group-fresh-lead-crawl/README.md)
  — campaign/queue/run orchestration with a freshness gate.
- [account-safety](features/account-safety/README.md) — account-runtime state
  machine, concurrency budgets, cooldown, safety boundaries.

## Not yet migrated

The rest of this domain's specs still live in the flat legacy folders
(`specs/facebook/`, `specs/lead/`, `specs/leadingest/`, `specs/comment/`,
`specs/outbound/`, `specs/copilot/`) and are tracked in
[`specs/SPEC_REGISTRY.json`](../../SPEC_REGISTRY.json). They migrate here in
later batches.

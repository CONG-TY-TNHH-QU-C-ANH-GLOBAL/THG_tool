@AGENTS.md

# THG Claude Code Entry Point

## Authoritative references

- Product:
  `specs/domains/facebook-sales-intelligence/roadmap.md`
- Runtime topology:
  `specs/domains/platform-foundation/features/runtime-topology/technical.md`
- Ownership domains:
  `specs/domains/` (index: each domain `README.md`; registry:
  `specs/SPEC_REGISTRY.json`, generated from node `SPEC_MANIFEST.json` files —
  workflow: `specs/registry/README.md`)
- Data planes:
  `docs/architecture/DATABASE_OWNERSHIP.md`
- Component ownership:
  `specs/domains/platform-foundation/decisions/component-structure-rules.md`
- Canonical helpers:
  `docs/engineering/CANONICAL_HELPERS.md`
- Workflow command contracts:
  `docs/ai/WORKFLOW_COMMANDS.md`
- Autopilot:
  `docs/ai/AUTOPILOT_QUEUE.md`
- Escalation:
  `docs/ai/ESCALATION_PLAYBOOK.md`
- Documentation:
  `docs/DOCS_GOVERNANCE.md`

OpenSpec (`openspec/changes/`) is the proposal lifecycle, never current runtime
authority.

## Priority

1. Correctness, security, tenant isolation, and data integrity.
2. Explicit task scope and acceptance criteria.
3. Merged specs and repository contracts.
4. Preserve untouched behavior.
5. Simplicity and readability.
6. Review-tool and aesthetic preferences.

## Required preflight

Before editing, report briefly:

- change class and track;
- exact files or boundary in scope;
- invariants preserved;
- validation required;
- stop conditions.

Do not output a long reasoning transcript.

## Diff contract

- Make the smallest coherent change.
- Every changed hunk maps to the task.
- Preserve unrelated formatting, imports, names, and behavior.
- Default to zero new comments.
- Stage explicit paths only.
- Never use `git add -A`.

## Data planes

- SQLite: local runtime/cache/outbox only.
- PostgreSQL: durable tenant-scoped SaaS source of truth.
- RAG: separate retrieval plane behind policy and ACL boundaries.

## Universal boundaries

- Every tenant mutation includes org ownership.
- Reuse canonical data owners.
- Composition roots wire; they do not own policy.
- Durable invariants belong in constraints or atomic transactions.
- Browser `content/` stays thin.
- Never implement checkpoint bypass, CAPTCHA solving, stealth/evasion,
  risk-triggered rotation, retry storms, or same-account concurrent crawl.
- Never weaken boundary, Sonar, CodeRabbit, lint, or CI configuration.

## Stop conditions

Stop and report when:

- baseline or working tree is not clean;
- canonical ownership is unclear;
- a small task requires an unapproved migration, auth, wire-contract, or
  cross-domain redesign;
- requested behavior conflicts with a merged spec;
- safe implementation requires broad or contested abstractions.

## Workflow selection

- `/focused-change`
- `/postgres-controlled-change`
- `/review-findings`
- `/clean-code-audit`
- `/finalize-pr`

Existing `/thg-*` commands and OpenSpec skills remain authoritative for their
specific workflows; their behavioral contracts live in
`docs/ai/WORKFLOW_COMMANDS.md`.

## Completion

Run `scripts/ai_preflight.sh` before controlled work and
`scripts/ai_validate.sh` before push.

Keep `.claude`, `.mcp.json`, `.env`, secrets, logs, scratch files, and generated
artifacts out of commits.

Push only when requested.
Never merge unless explicitly requested.

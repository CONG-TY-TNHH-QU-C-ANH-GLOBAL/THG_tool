# THG AutoFlow — Cross-Agent Instructions

THG AutoFlow is a multi-tenant Facebook Sales Intelligence and Automation
platform. It is not a generic scan-all scraper or spam automation system.

This file contains instructions shared by all coding agents.
Claude-specific workflows live in `CLAUDE.md` and `.claude/`.

## Ownership domains

Specifications are organized by ownership domain under `specs/domains/`
(Domain → Experience → Technical Feature → implementation/decisions/evidence/
runbooks). `specs/SPEC_REGISTRY.json` is the machine-readable authority index.

- `specs/domains/facebook-sales-intelligence/` — product domain: fresh-lead
  discovery, lead management, engagement approval, sales copilot.
- `specs/domains/knowledge-platform/` — product platform: org-scoped business
  knowledge, calibration, retrieval.
- `specs/domains/platform-foundation/` — platform: runtime topology, tenant
  ownership, store architecture, browser connector, AI cost controls,
  data platform, workspace UI.

## Authoritative references

Read only documents relevant to the task:

- Product direction:
  `specs/domains/facebook-sales-intelligence/roadmap.md`
- Runtime topology (current runtime authority, script-enforced):
  `specs/domains/platform-foundation/features/runtime-topology/technical.md`
- Platform vocabulary:
  `specs/domains/platform-foundation/DOMAIN.md`
- Data planes:
  `docs/architecture/DATABASE_OWNERSHIP.md`
- Account safety constitution:
  `specs/domains/facebook-sales-intelligence/features/account-safety/technical.md`
- Component structure:
  `specs/domains/platform-foundation/decisions/component-structure-rules.md`
- Canonical helper layer:
  `docs/engineering/CANONICAL_HELPERS.md`
- Documentation governance:
  `docs/DOCS_GOVERNANCE.md`
- AI workflow:
  `docs/ai/AUTOPILOT_QUEUE.md`
  and `docs/ai/ESCALATION_PLAYBOOK.md`

OpenSpec (`openspec/`) is the change/proposal lifecycle: `openspec/changes/`
holds proposed deltas. Nothing under `openspec/` is current runtime authority.

## Core product invariants

- Every tenant-facing record and mutation is org-scoped.
- Non-platform users with `org_id = 0` cannot access tenant APIs.
- Business profile and customer segments drive classification.
- If a workspace has no business calibration yet, Dashboard Chat and Telegram
  must ask for the organization's identity, offer, target role, positive
  signals, and reject signals before creating Facebook crawl jobs.
- Do not hardcode a single industry or vertical.
- Do not run broad scan-all jobs by default.
- Outbound actions are approval-required unless an explicit org/campaign policy
  enables execution.
- Browser automation remains visible and operator-observable; workers attach to
  the selected account's visible workspace Chrome session.
- No hidden browser pools and no duplicate API services in production.
- Login walls and checkpoints return `human_required`.
- Never implement CAPTCHA solving, checkpoint bypass, stealth/evasion,
  risk-triggered account rotation, retry storms, or same-account concurrent
  crawling.
- AI-generated business claims must be grounded in verified org data.
- Do not generate AI images; use real user-provided media only.

## Data planes

- SQLite owns local runtime, cache, and outbox state.
- PostgreSQL owns durable tenant-scoped SaaS platform state.
- RAG/Knowledge uses a separate retrieval plane with policy and ACL boundaries.
- Cross-plane movement uses explicit events, outbox, and idempotency.
- Never blur the planes through hidden shared tables.

## Engineering boundaries

- Reuse canonical data and store owners.
- Composition roots wire dependencies; transport handlers remain thin.
- Durable invariants belong in database constraints or atomic transactions.
- Service policy, persistence, transport, and browser DOM logic remain separate.
- Browser `content/` is a thin bridge; platform-specific behavior belongs under
  `platforms/<platform>/`.
- Do not create speculative interfaces, frameworks, helper soup, or generic
  dumping-ground packages.
- Preserve behavior outside the approved task.
- Avoid unrelated formatting and noisy diffs.

## Validation

Use the repository workflows:

```bash
bash scripts/ai_preflight.sh
bash scripts/ai_validate.sh
```

# THG AutoFlow - Claude Code Guide

Claude Code should treat `AGENTS.md` as the short instruction file and
`specs/FACEBOOK_BUSINESS_ANALYSIS_AUTOMATION_PLAN.md` as the detailed product
and implementation plan.

## Read First

1. `specs/FACEBOOK_BUSINESS_ANALYSIS_AUTOMATION_PLAN.md`
2. `openspec/root-architecture.md`
3. `specs/ROOT_ARCHITECTURE.md`
4. `AGENTS.md`

## Product North Star

Build toward:

> AI Facebook Sales Intelligence Workspace for each business.

The system is not a fixed scraper and not a spam automation tool. It learns each
organization's business, customer segments, sources, market signals, and sales
strategy. Facebook automation is used after analysis, with visible browser
sessions and human approval for risky outbound actions.

The platform should include a Workspace Skill Designer: admins describe a
Facebook-related business workflow in natural language, and the system turns it
into a validated blueprint of data entities, classifiers, dashboard views,
actions, and approval rules. Treat HR/recruitment, POD sourcing, sales lead
discovery, and similar verticals as playbooks on shared primitives, not
hardcoded scrapers.

## Current Stack

- Go backend with Gofiber.
- Next.js frontend in `frontend/`.
- SQLite for the current MVP database.
- Browser automation through persistent per-account workspaces.
- Prompt-scoped jobs through the job/task pipeline.

Do not reintroduce legacy `internal/server/static/` production UI files.

## Highest-Priority Direction

The next product work should implement:

1. Org-scoped business profiles.
2. Customer segment definitions and AI suggestions.
3. Market signals beyond simple leads.
4. Source discovery and source quality scoring.
5. Opportunity map and strategy recommendations.
6. Campaign approval and safe outbound execution.
7. Outcome learning.
8. Workspace Skill Designer and blueprint validation.
9. HR/recruitment reference blueprint.

## Hard Rules

- Every tenant feature needs `org_id` ownership checks.
- Business profile and customer segments must drive AI classification.
- Do not hardcode one industry.
- User-designed skills must compile to validated blueprints and approved
  primitives; do not execute arbitrary LLM-generated code in production.
- Do not run broad scan-all behavior.
- Browser automation must be observable.
- Default outbound automation to approval-required.
- Return `human_required` on login wall/checkpoint.
- Do not generate AI images. Use real uploaded files/images only.

## Verification

Run the relevant checks after changes:

```powershell
go test ./...
go vet ./...
npm --prefix frontend run build
```

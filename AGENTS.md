# THG AutoFlow - Agent Instructions

This repository is evolving from a Facebook scraper into a multi-tenant
Facebook Sales Intelligence and Automation workspace.

Start here before making changes:

1. `specs/FACEBOOK_BUSINESS_ANALYSIS_AUTOMATION_PLAN.md`
2. `openspec/root-architecture.md`
3. `specs/ROOT_ARCHITECTURE.md`
4. `frontend/src/modules/autoflow/`
5. `internal/handlers/facebook_crawl/handler.go`
6. `internal/ai/business.go`
7. `internal/ai/universal.go`

## Current Product Direction

Do not treat the product as a generic "scan all Facebook groups" scraper.

The product direction is:

> AI Facebook Sales Intelligence Workspace for each business.

Each organization provides its brand, services, target customers, private files,
pricing, tone, reject rules, and approval policy. Automation then uses the org's
own logged-in Facebook browser workspaces to discover sources, classify market
signals, suggest customer segments, recommend campaigns, and draft or execute
approved outreach.

Automation is the execution layer. Business analysis is the value layer.

The platform should also support a Workspace Skill Designer: admins describe a
Facebook-related business workflow in natural language, and the system proposes
a validated blueprint for entities, classifiers, views, actions, and approval
rules. This is a Claude-style workflow designer, not arbitrary code execution.

## Current Architecture

- Backend: Go, Gofiber, SQLite for current MVP.
- Frontend: Next.js app in `frontend/`.
- Main backend binary: `cmd/scraper/main.go`.
- Worker binary: `cmd/worker/main.go`.
- Browser runtime: persistent per-account browser sessions via workspace,
  session, livesession, runtime, and browser packages.
- Task execution: prompt-scoped jobs, not broad fixed scrapes.
- Auth: JWT access tokens plus HTTP-only refresh cookie.

Legacy embedded static frontend files under `internal/server/static/` are not
the production UI and must not be reintroduced.

## Core Invariants

- Every tenant-facing record must be org-scoped.
- Non-superadmin users with `org_id=0` must not access tenant APIs.
- One workspace can manage multiple Facebook accounts.
- One Facebook account maps to one persistent browser profile.
- Browser automation must remain observable through the dashboard Browser view.
- Business profile and customer segments must drive classification.
- Do not hardcode logistics, recruitment, POD, or any other vertical into the
  crawler.
- Domain features such as HR/recruitment, POD sourcing, sales lead discovery, or
  support monitoring should be playbooks/blueprints built on shared primitives.
- Do not run broad scan-all jobs by default. A job needs a target URL, search
  query, source, or campaign context.
- Default outbound comments, inbox messages, and posts to draft or
  approval-required.
- If Facebook shows login wall/checkpoint, return `human_required`.
- Do not generate AI images. Only use real user-uploaded files/images.

## Important Code Areas

- Product intelligence plan: `specs/FACEBOOK_BUSINESS_ANALYSIS_AUTOMATION_PLAN.md`
- Root system boundaries: `openspec/root-architecture.md`
- Frontend app: `frontend/src/modules/autoflow/`
- API routes: `internal/server/api.go`
- Auth and onboarding: `internal/server/auth_handlers.go`,
  `internal/server/google_auth.go`, `internal/server/onboarding_handlers.go`
- Business profile: `internal/ai/business.go`
- Universal classifier/comment/inbox: `internal/ai/universal.go`
- Prompt-scoped crawl handler: `internal/handlers/facebook_crawl/handler.go`
- Task schema: `internal/jobs/model.go`
- App task/leads store: `internal/store/app_store.go`
- Org/account/user store: `internal/store/`

## Implementation Guidance

When adding business-analysis features, prefer this sequence:

1. Org-scoped business profile.
2. Customer segments.
3. Market signals.
4. Source catalog and discovery.
5. Strategy recommendations.
6. Campaign approval.
7. Outcome learning.
8. Workspace Skill Designer and blueprint validation.
9. HR/recruitment reference blueprint.

For Go changes, run:

```powershell
go test ./...
go vet ./...
```

For frontend changes, run:

```powershell
npm --prefix frontend run build
```

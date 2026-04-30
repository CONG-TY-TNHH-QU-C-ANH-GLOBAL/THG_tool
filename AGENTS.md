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
- Dashboard data is expected to be API-backed. Do not reintroduce fake cards for
  Settings, Staff, AI Agents, Billing, Posting, Commenting, Inbox, Leads, or
  Data Private.
- Data Private is now a knowledge hub:
  - manual uploads are stored in `private_files`
  - business memory is stored per org in `user_context` keys prefixed with
    `org:{id}:`
  - external data connectors are stored in `data_sources`
  - Google Sheets quick sync reads published/exportable CSV data into AI context
  - Google Drive sources are registered as `needs_auth` until read-only Drive
    OAuth is implemented; do not fake Drive media sync

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
- Explicit auto/execution mode is allowed only for an org/campaign/prompt that
  asks for it. Even then, outbound must pass org-scoped dedup, cooldown, and
  conversation-thread guardrails before entering `approved` outbox state.
- AI must treat inbox as customer-service state, not one-shot blasting: if a
  lead replied, answer the latest reply with thread context; if they have not
  replied, do not keep sending repeated inbox messages inside the cooldown.
- If Facebook shows login wall/checkpoint, return `human_required`.
- Do not generate AI images. Only use real user-uploaded files/images.
- External business data connectors must be org-scoped, read-only by default,
  auditable, and explicit. Never scan a user's entire Google Drive.
- AI context should be summarized/retrieved from org data sources; do not dump
  large raw files or sheets directly into prompts.

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
- Data Private and connectors:
  - `internal/server/autoflow_handlers.go`
  - `internal/server/data_connector_handlers.go`
  - `internal/store/data_sources.go`
  - `frontend/src/modules/autoflow/components/views/DataPrivateView.tsx`
  - `frontend/src/modules/autoflow/components/data/`
  - `frontend/src/modules/autoflow/services/dataSourceService.ts`
- Outbound automation guardrails:
  - `cmd/scraper/main.go` wires `search_groups`, `comment_all_leads`,
    `inbox_all_leads`, and `create_job_post` into real jobs/outbox rows.
  - `internal/store/store.go` owns `CanQueueOutboundForOrg`, conversation
    thread state, and org-scoped lead retrieval for automation.
  - Dashboard and Telegram prompts should call `ProcessPromptForOrg` so account
    mapping, data context, and outbound actions stay tenant-scoped.

## Implementation Guidance

When adding business-analysis features, prefer this sequence:

1. Org-scoped business profile.
2. Private data connectors and business memory.
3. Customer segments.
4. Market signals.
5. Source catalog and discovery.
6. Strategy recommendations.
7. Campaign approval.
8. Outcome learning.
9. Workspace Skill Designer and blueprint validation.
10. HR/recruitment reference blueprint.

For Go changes, run:

```powershell
go test ./...
go vet ./...
```

For frontend changes, run:

```powershell
npm --prefix frontend run build
```

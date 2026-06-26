# THG AutoFlow - Agent Instructions

This repository is evolving from a Facebook scraper into a multi-tenant
Facebook Sales Intelligence and Automation workspace.

**Before implementing, follow `CLAUDE.md`, especially:**
- **Hard Rules**
- **Engineering Guardrails** (200-line rule, no god files, `scripts/check_file_size.py`, `docs/PR_CHECKLIST.md`)
- **Component structure** — before adding/moving files, classify the component owner and check `specs/COMPONENT_STRUCTURE_RULES.md` (triage: `specs/COMPONENT_HOTSPOTS.md`; guard: `scripts/check_component_structure.py`)
- **Verification**

Start here before making changes:

1. `specs/PRODUCTION_FLOW.md` — current production wiring + helper layer (read first if you're changing auth, outbound, browser, or AI prompts)
2. `specs/STRUCTURAL_REFACTOR_PLAN.md` — open structural debt (auth boundary, worker path drop, file-creation principle); check before adding a package or starting a rename
3. `specs/BROWSER_GATEWAY_AND_FACEBOOK_AUTOMATION_VISION.md`
4. `specs/SALES_VOICE_AUTOMATION_AND_DATA_PRIVATE_ENTERPRISE_PLAN.md`
5. `specs/PRODUCTION_DATABASE_MIGRATION_PLAN.md`
6. `specs/FACEBOOK_BUSINESS_ANALYSIS_AUTOMATION_PLAN.md`
7. `openspec/root-architecture.md`
8. `specs/ROOT_ARCHITECTURE.md`
9. `frontend/src/modules/autoflow/`
10. `internal/handlers/facebook_crawl/handler.go`
11. `internal/ai/business.go`
12. `internal/ai/universal.go`

## Current Product Direction

Do not treat the product as a generic "scan all Facebook groups" scraper.

The product direction is:

> AI Facebook Sales Intelligence Workspace for each business.

Each organization provides its brand, services, target customers, private files,
pricing, sales voice, reject rules, and approval policy. Automation then uses
the org's own logged-in Facebook browser workspaces to discover sources,
classify market signals, suggest customer segments, recommend campaigns, and
execute approved outreach. Drafts are an internal safety fallback, not the
primary product experience.

Automation is the execution layer. Business analysis is the value layer.

The platform should also support a Workspace Skill Designer: admins describe a
Facebook-related business workflow in natural language, and the system proposes
a validated blueprint for entities, classifiers, views, actions, and approval
rules. This is a Claude-style workflow designer, not arbitrary code execution.

## Current Architecture

- Backend: Go, Gofiber, SQLite for current MVP.
- Production database target: PostgreSQL. SQLite is only for local MVP/dev
  until `specs/PRODUCTION_DATABASE_MIGRATION_PLAN.md` is implemented.
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
- AI must treat inbox CHANNEL DISCIPLINE as customer-service-style, not
  one-shot blasting: if a lead replied, answer the latest reply with thread
  context; if they have not replied, do not keep sending repeated inbox
  messages inside the cooldown.
- First-touch outreach via the `inbox_all_leads` skill IS sales semantics —
  it defaults to `score_filter=hot` to target qualified, never-contacted
  leads. The customer-service discipline above applies AFTER first-touch:
  the `awaiting_reply_cooldown` gate prevents repeat sends on the same
  thread. Customer-service reply-to-inbound-message uses a different code
  path (thread with `last_inbound_at > last_outbound_at` is gated as
  `lead_replied=Allowed`, so the operator can respond, but the bulk skill
  itself is not a CS triage tool).
- If Facebook shows login wall/checkpoint, return `human_required`.
- Do not generate AI images. Only use real user-uploaded files/images.
- External business data connectors must be org-scoped, read-only by default,
  auditable, and explicit. Never scan a user's entire Google Drive.
- AI context should be summarized/retrieved from org data sources; do not dump
  large raw files or sheets directly into prompts.

## Helper Layer (Phase 1, 2026-05-03)

Use these instead of writing org checks / session writes from scratch:

- `store.GetAccountForOrg(id, orgID)` — data-layer org boundary. The
  unscoped `GetAccount(id)` is reserved for worker code that already
  proved org ownership; tenant-facing handlers must use the org variant.
- `(s *Server) requireAccountForOrg(c, accID, orgID)` — fiber HTTP guard.
  Writes 404 on miss; caller returns the error.
- `(s *Server) requireAccountForOrgWS(orgID, role, accID)` — WebSocket
  guard, honours `IsPlatformUser` for founder/superadmin.
- `(s *Server) rejectIfFacebookProfileMismatch(c, ctx, acc, fbUID, orgID)`
  — call before mutating any FB identity. Writes 409 + records
  `local_error` session row on mismatch.
- `(s *Server) applyConnectorIdentity(ctx, snap)` — single pipeline that
  upserts browser_sessions, sets the FB identity, and flips the account
  active flag. Heartbeat / chrome-status / screenshot all funnel here.
- `store.LocalSessionStatus` enum + `store.LocalSessionStatusFromStream`
  / `store.LocalFacebookNotReady` — typed session lifecycle states.
- `store.AppStore.RecordLocalSession(ctx, accID, orgID, status, errMsg)`
  — upsert browser_sessions row with the typed enum.
- `store.ConnectorOwnsAccountStream(orgID, agentID, accID)` — call before
  trusting any work an agent attributes to an account.
- `clampPresenceFields(*store.AgentPresence)` — clamp every connector
  string before it lands in the DB; size limits in `input_limits.go`.
- `cfg.MustValidateProductionSecrets()` — boots refuse to start if
  `APP_ENV=production` and JWT_SECRET / ENCRYPTION_KEY are missing.
- `ai.sanitizeForPrompt(value, maxRunes)` — strip control chars + clamp;
  use it whenever you mix user-controlled text into an LLM prompt.
  Wrap user data in `BEGIN USER_DATA … END USER_DATA` markers.
- `store.QueueOutboundForOrg(msg, requestedAuto, cooldown)` — canonical
  write path for AI / agent / Telegram outbound. Atomic guard +
  store-layer approval policy + UNIQUE index race protection. Use
  this, not `InsertOutboundMessage`, from any LLM-driven path.
- `store.IsAutoOutboundEnabledForOrg(orgID)` — single source of truth
  for whether the org has opted into auto-execute. Backed by
  `org:{id}:outbound_mode` user_context key, admin-only.
- `workspace.AcquireProfileLock(profileDir)` — cross-process exclusive
  claim on a Chrome `--user-data-dir`. Held for the lifetime of the
  container; released on Stop / StopAll / failed Start. Don't bypass.
- `RestartController` per-account in-flight + 30 s cooldown debounce —
  multiple OnUnhealthy calls for the same account no-op while a
  restart is in progress.
- `session.CheckpointVerifier` — server.go wires
  `workspaceCheckpointVerifier` so `ResolveCheckpoint` cannot transition
  back to ready while Chrome is still on a verification page. Returns
  `*ErrCheckpointStillActive` → handlers map to HTTP 409
  (`CHECKPOINT_STILL_ACTIVE`).
- `skills.Registry` — open-prompt agent catalog. Built-ins registered
  in `cmd/scraper/skills_register.go` from main.go's existing action
  handler closure. The agent uses `skills.OpenAITools(reg.EnabledFor)`
  for the LLM tool list and `reg.Execute` for the typed validation +
  per-org enablement check + audit logging in `skill_executions`.
- `store.SetOrgSkillEnabled / GetOrgSkillConfig / SetOrgSkillConfig` —
  admin-only writes for the per-org skill blueprint and per-skill
  config JSON. The dashboard chat and Telegram bot both go through
  `Agent.ProcessPromptForOrgWithAccount`, so adding a skill anywhere
  exposes it to both surfaces simultaneously.
- New skills `scan_fanpage_inbox`, `care_fanpage`, `post_to_profile`
  ship as scaffolds today; live Chrome-driving execution lands after
  Phase 4 (CDP whitelist).

Adding a new endpoint that touches accounts? Use the helpers. If you
find yourself writing `if acc.OrgID != orgID` by hand, you're working
against the boundary — switch to `requireAccountForOrg` instead.

## Important Code Areas

- Product intelligence plan: `specs/FACEBOOK_BUSINESS_ANALYSIS_AUTOMATION_PLAN.md`
- Production flow + helpers: `specs/PRODUCTION_FLOW.md`
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

## Documentation Governance

`AGENTS.md` is a thin cross-agent entrypoint — do not put long specs, plans, or
debt lists here. For where docs live and how to add them, see
`docs/DOCS_GOVERNANCE.md` and `docs/INDEX.md`. Do not create new root `.md` files
(only `README.md`, `AGENTS.md`, `CLAUDE.md`, `SPEC_GOVERNANCE.md` are permitted at
root); new docs go under the correct `docs/*` category. `scripts/check_docs_governance.sh`
enforces this (warn-only for legacy root docs).

## Autopilot queue

Use `docs/ai/AUTOPILOT_QUEUE.md` as the stable queue index.
Per-item status lives in `docs/ai/queue/items/**/*.md` (grouped by domain; discovered recursively).
Do not edit the central queue file in normal work PRs.

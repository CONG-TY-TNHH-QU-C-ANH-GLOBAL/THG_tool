# Domain Model — THG Platform

**Status:** Living document. Updates require coordination — see *How to update* at the bottom.
**Audience:** Mandatory reading for backend, frontend, AI agents, ops, docs, and commits. Synonyms are rejected on review.

The THG platform spans multiple services (Facebook Automation today; Taobao and 1688 on the roadmap), multiple workers, and multiple data stores. Without a shared vocabulary, the same concept gets named differently in each layer — and the same name gets reused for different concepts. Both are bugs.

This document defines the canonical terms.

---

## Entities

### Platform

- **Definition:** The THG product as a whole. The single SaaS deployment that hosts services for many users and many workspaces.
- **Owner:** Platform layer (`frontend/src/platform/`; backend `internal/platform/` once PR 2 lands).
- **Identity:** Singular. One platform instance per deployment.
- **Lifecycle:** Always live.
- **Forbidden synonyms:** "the app", "the system" — use **Platform**.

### User

- **Definition:** An authenticated individual. Owns credentials (email/password/Google). NOT scoped to a workspace.
- **Owner:** Platform auth (`internal/auth/`).
- **Identity:** `user.id` (string at all contract boundaries; numeric internal storage is an implementation detail).
- **Lifecycle:** `signed_up → active → disabled`.
- **Relationships:** Has 0+ Memberships across Workspaces. Has a platform role (regular | founder | superadmin).
- **Forbidden synonyms:** account, member, tenant, customer.

### Service

- **Definition:** A distinct automation domain offered by the platform. Each service is independently deployable / removable. Examples: Facebook Automation, Taobao Automation, 1688 Order Automation.
- **Owner:** Platform service registry (FE: `frontend/src/platform/services/registry.ts`; BE post-PR 2: `internal/platform/services/`).
- **Identity:** `slug` (string, e.g. `"facebook"`). Never a numeric ID. **The slug is permanent and immutable** — it is the service's identity, not its name. A service is never renamed. Marketing/UX renames change `descriptor.publicLabel`; the slug stays forever. Renaming a slug is a delete + create, never an edit.
- **Lifecycle:** `ServiceStatus` per (user, service): `available | unavailable | suspended`.
- **Relationships:** Has 1+ Workspaces (currently 1 per user; multi-workspace is a capability flag).
- **Forbidden synonyms:** module, app, feature, product.

### Workspace

- **Definition:** A user's operational space within a single service. Holds the service's per-user state: connected accounts, leads, sessions, configuration, team memberships.
- **Owner:** The Service that hosts it.
- **Identity:** `workspaceId` (**string**, always paired with a service slug). Numeric `org_id` in the SQLite/Postgres layer is the storage representation, NOT the contract identity.
- **Lifecycle:** `WorkspaceState`: `none | pending_invite | initializing | ready | suspended | billing_blocked`.
- **Relationships:** Belongs to one Service. Has 1+ Memberships. Has 0+ Sessions. Has 0+ AutomationJobs.
- **Forbidden synonyms:** organization, org, tenant, account, group.

### Membership

- **Definition:** A User's role within a specific Workspace. Resolves access and per-workspace permissions.
- **Owner:** Platform (the resolver consults backend RBAC).
- **Identity:** `(userId, workspaceId)` tuple.
- **Lifecycle:** `invited → active → (removed | suspended)`.
- **Relationships:** User ↔ Workspace.
- **Forbidden synonyms:** role-on-its-own (role is the per-platform attribute on User; Membership is the per-workspace relation), assignment.

### Capability

- **Definition:** A user-derived feature flag that gates UX/code paths within a Service. Examples: `browserAutomation`, `aiAgents`, `multiWorkspace`. May expand to: `plan`, `region`, `betaAccess`, `workerAvailability`.
- **Owner:** Service module's `resolveCapabilities(user)`. Post-PR 2: backend resolver under `internal/platform/services/`.
- **Identity:** A key in the capabilities object.
- **Lifecycle:** N/A — derived state.
- **Forbidden synonyms:** permission (permission is RBAC inside Membership), feature flag without qualifier (a platform-wide feature flag is different from a per-user Capability).

### Session

- **Definition:** A live operational connection to an external system, owned by a Workspace. Today: a Facebook Chrome session for a connected account. Tomorrow: a Taobao session, a 1688 session, etc.
- **Owner:** The Service module that operates external systems. Post-PR 3, may live in a shared browser orchestration boundary.
- **Identity:** `sessionId` (string).
- **Lifecycle:** `booting → ready → degraded → checkpoint_required → terminated`.
- **Relationships:** Belongs to one Workspace. Executes on 1+ Workers (may rotate).
- **Forbidden synonyms:** account (account is the FB persona; Session is the live binding), browser instance, profile, tab.

### Worker

- **Definition:** A process that executes Service work. Today: `cmd/scraper`, `cmd/worker`, `services/agent-brain`, the Chrome Extension on user's local machine. Tomorrow: per-service workers.
- **Owner:** Operations / infra layer.
- **Identity:** `workerId` (string, often hostname + PID + start time).
- **Lifecycle:** `booting → ready → unhealthy → terminated`.
- **Relationships:** Executes on Sessions. Claims AutomationJobs.
- **Forbidden synonyms:** agent (we use Agent specifically for the LLM-driven orchestrator — a Worker is the process; an Agent is the brain inside a Worker), executor, runner.

### AutomationJob

- **Definition:** A unit of work executed by a Worker. Has explicit intent (`search_groups`, `comment_all_leads`, etc.), explicit target (URL/query/account), and explicit lifecycle state.
- **Owner:** Job pipeline (`internal/jobs/` today; `packages/core-queues` post-monorepo migration).
- **Identity:** `jobId` (string at contract boundary).
- **Lifecycle:** `queued → claimed → running → (succeeded | failed | timed_out | cancelled)`.
- **Relationships:** Belongs to a Workspace. Claimed by a Worker. May execute on a Session.
- **Forbidden synonyms:** task (task is a colloquial parent — a "task" may decompose into many AutomationJobs).

---

## Identity rules

1. Every entity ID is a **string** at the contract boundary (HTTP, JSON, cross-service messages). Numeric IDs are an internal-storage detail.
2. Identifiers are immutable. A renaming/re-keying creates a new entity; the old one becomes `terminated`.
3. Composite identity (e.g. Membership = `(userId, workspaceId)`) is allowed only when the tuple is the canonical primary key.
4. URLs that reference an entity MUST include its identity explicitly. No sugar URLs like `/services/facebook/current` or `/workspace/default` — always `/services/:slug/workspaces/:workspaceId`.
5. **ID prefix convention.** Every ID at a contract boundary carries a type prefix. Adopted from day 1 (PR 1 forward) — do not postpone:

   | Entity | Prefix | Example |
   |---|---|---|
   | User | `usr_` | `usr_42` |
   | Workspace | `ws_` | `ws_42` |
   | Session | `ses_` | `ses_abc123` |
   | Worker | `wrk_` | `wrk_pod7-3491` |
   | AutomationJob | `job_` | `job_2026051401` |
   | Membership | `mbr_` | `mbr_42x9` |
   | Service | (uses `slug`) | `facebook` |

   Storage may keep raw numerics for now; the prefix is applied at the adapter/projection boundary. Numeric IDs MUST NOT cross any HTTP, JSON, or cross-service boundary in new code.

## Storage model is NOT the domain model

This is one of the strongest anti-spaghetti rules in the project. Internalise it:

- **Database tables are a persistence concern.** Their shape is driven by storage efficiency, indexing, migration cost, engine choice. `orgs`, `task_leads`, `browser_sessions` are storage artefacts.
- **Domain contracts are a semantic concern.** Their shape is driven by what the business means. `Workspace`, `AutomationJob`, `Session` are domain entities.
- **Resolvers + adapters mediate between them.** An adapter reads storage rows and produces domain entities. A resolver answers a semantic question using domain entities. Neither layer lets the other's shape leak through.

Consequences:

- A storage column rename (or a switch from SQLite to Postgres, or sharding the `orgs` table) MUST NOT change any domain contract.
- A new domain field (a resolved capability, a computed workspace state) MAY have no single backing column — it's synthesised by a resolver.
- `json.Marshal(dbRow)` for a cross-boundary response is a bug. The handler transforms storage → domain contract first.
- The grandfathered `org_id` column stays in storage; it never appears in a new contract, response field, or log line — it becomes `workspaceId` at the adapter boundary.

If you ever find a DB column name in an HTTP response, a frontend type, or a cross-service message — that's a storage leak. Fix the adapter.

## Resolution pipeline

Information flows in one direction: raw storage → adapters → domain entities → resolvers → projected contract → presentation → UI. UI never reaches back to a raw field; each layer above is opaque to the layer below.

```
Raw storage  →  Domain entities  →  Service resolvers  →
   PlatformService contract  →  Presentation resolver  →  UI
```

Detailed pipeline + reverse-direction rule: [frontend/src/platform/BOUNDARIES.md § Resolution pipeline](../../../frontend/src/platform/BOUNDARIES.md).

---

## Relationships diagram

```
Platform 1 ─── * User
User 1 ─── * Membership * ─── 1 Workspace
Service 1 ─── * Workspace
Workspace 1 ─── * Session
Workspace 1 ─── * AutomationJob
Session 1 ─── * Worker
Worker 1 ─── * AutomationJob
Capability  (resolved per User × Service; no entity row)
```

---

## Forbidden synonyms (cheat sheet)

| Wrong (legacy / colloquial) | Right |
|---|---|
| organization / org / tenant / account (the workspace concept) | **Workspace** |
| account (the human) / customer / member-as-noun | **User** |
| member-as-relation / staff | **Membership** |
| module / app / feature / product (when describing a service line) | **Service** |
| executor / runner / process | **Worker** |
| task (when meaning a single unit) | **AutomationJob** |
| browser session / profile / tab (when meaning the live binding) | **Session** |
| feature flag (when user-derived) | **Capability** |
| role (when workspace-scoped) | **Membership.role** |
| current workspace / default workspace / my workspace | The explicit `workspaceId` from the URL or contract |

Legacy code may keep old names if changing them is too invasive, but **new code must use the canonical term**. The Go monolith currently uses `org_id` internally for storage — that storage name is grandfathered, but every NEW contract field, HTTP response field, JSON envelope, log line, and frontend type must say `workspaceId`.

---

## How to update this document

This file is **append-mostly**:

- **Adding a new entity** is allowed when (and only when) the entity exists in code and has gone through at least one PR review where its definition was discussed.
- **Renaming an existing entity** is a project-wide rename event — coordinate before changing any name here. Open a tracking issue first.
- **Adding a forbidden synonym** to the cheat sheet is encouraged — every observed drift is a future bug avoided.

When in doubt about a term: search this file first. If the term isn't here, you're inventing one — pause and use the closest canonical term instead.

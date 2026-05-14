# Frontend Platform Boundaries

**This file is mandatory reading for any contributor touching `frontend/src/platform/` or `frontend/src/modules/*`.**

The frontend is split into two layers with strict ownership. Violating the boundary breaks the ability to add a second service (Taobao, 1688, ...) without rewriting the platform.

---

## Layer 1 — Platform (`frontend/src/platform/`)

The platform owns the shell, the identity, the routing, and everything that is independent of a specific service.

### Owns

- **Auth shell** — login, signup, session restore, logout. Public landing page.
- **Navigation** — top nav with user identity (`user.name` top right), service switcher, breadcrumb root.
- **Routing** — every URL path mapping. The Next.js `app/(platform)/services/[slug]/...` routes belong here. A service module never declares its own route paths.
- **Notifications** — global toast/notification slot (future).
- **Service registry** — the typed contract for a service, the registry primitives (`registerService`, `getService`, `listServices`), and the platform-side bootstrap that calls them.
- **Global UX** — theme toggle, language switch, density switch, error boundary, suspense boundary.
- **Global state boundaries** — `authStore`, `roleStore`, language store, theme provider.

### MUST NOT own

- Service data values (the platform does not know "Facebook" — only "some registered service").
- Workspace lifecycle (which workspace the user is in, what its state is).
- Business logic for any specific service.

---

## Layer 2 — Service modules (`frontend/src/modules/<service>/`)

Each service is a self-contained module that exposes a **pure-data contract** to the platform.

### Owns

- **Business logic** — Facebook scraping, comment automation, lead pipelines, browser sessions, AI agents (FB-specific knowledge).
- **Service-specific state** — workspace identity, account picker state, crawl queue, FB session status.
- **Workflows** — the screens that compose a feature flow (e.g. create workspace, lead detail, browser stream).
- **Workspace internals** — sidebar with FB-specific tabs (leads, browser, inbox, posting, commenting, ...), tab routing within a workspace.
- **Service APIs** — HTTP client calls scoped to the service's endpoints.

### MUST NOT own

- The top nav (lives in PlatformShell).
- User identity rendering (PlatformShell shows `user.name`; modules never re-render it).
- URL paths outside the module's own subtree (`/services/<slug>/...`). The platform router decides which view to mount where.
- Logic that infers cross-cutting state from FK ids (`org_id === 0` etc.) — see "Forbidden" below.

---

## Service Module Contract

A service module exports a single pure-data value of type `ServiceModule`:

```ts
export const facebookServiceModule: ServiceModule = {
  slug: "facebook",
  serviceVersion: 1,
  label: "Facebook Automation",
  capabilities: { multiWorkspace: false, browserAutomation: true, aiAgents: true },
  views: {
    createWorkspace: CreateFacebookWorkspace,
    workspace:       FacebookWorkspaceApp,
  },
  resolveStatus: () => "available",
  resolveWorkspace: (user) =>
    user && user.org_id > 0
      ? { state: "ready", workspaceId: String(user.org_id) }
      : { state: "none" },
}
```

**Importing this value MUST NOT register it.** Registration is a separate explicit step performed by `frontend/src/platform/services/bootstrap.ts` (which the platform calls once). This keeps imports pure, tree-shaking clean, and tests deterministic.

`resolveWorkspace` is the **only** place where a service-specific proxy (`user.org_id`) is allowed. Everywhere else in the codebase reads `service.workspaceState` and `service.workspaceId` from the contract.

---

## Forbidden

These patterns are rejected on review and must be refactored before merge:

1. **Platform layer imports module internals.** `frontend/src/platform/**` must never `import` from `frontend/src/modules/**`. The bootstrap file (`bootstrap.ts`) is the single exception — it imports module contracts as pure data.

2. **Service module mutates platform routing.** A module never calls `router.push("/services/foo")` to navigate to another service or to a platform-level path. It only navigates within its own subtree.

3. **Proxy/inferred semantic state.** No `user.org_id === 0` → "needs onboarding". No `length === 0` → "no data". Use the explicit field on the service contract. If the field doesn't exist, add it — never proxy. See [feedback_deterministic_boundaries (in memory)](../../../../C:/Users/ACER/.claude/projects/d--THG-THG-sale/memory/feedback_deterministic_boundaries.md).

4. **"Onboarding" vocabulary for service workspace creation.** "Onboarding" is reserved for platform account signup. Service workspace creation is "Create X Workspace" or "Initialize X". No screen, route, or copy may use "onboarding" in a service context.

5. **Service slug as a union type.** `slug: "facebook" | "taobao"` is rejected. Use `slug: string`. Services are data, not a compile-time enum.

6. **Side-effect registration on import.** A module exporting `registerService(facebookServiceModule)` at top level is rejected. Export the contract value; let bootstrap register it.

7. **Workspace id as `number`.** Use `string`. Future UUIDs, snowflakes, and external provider IDs depend on this.

---

## Route ownership

The route ownership rules are part of the boundary and not optional.

**Platform routes (`frontend/app/(platform)/...`) MAY:**

- Mount service routes via Next.js dynamic segments (`[slug]`, `[workspaceId]`).
- Perform auth checks (redirect unauthenticated users, gate by role).
- Resolve workspace identity (read `service.workspaceState` and decide what to render: workspace, create, suspended, etc.).
- Provide `ServiceContext` to descendant views.
- Wrap descendants with `ServiceBoundary` for per-service Suspense and ErrorBoundary.

**Service modules MAY NOT:**

- Mutate the platform router root state (no `router.push('/superadmin')` from inside a module).
- Declare or own URL path strings outside their service subtree (`/services/<slug>/...`). A module that pushes to `/services/other-slug/...` is wrong.
- Navigate outside their service scope directly. To leave the service, return control to the platform (e.g. via `router.push('/services')` is the one exception — back to the directory, not into another module).
- Render their own top nav, language switch, theme toggle, or service switcher (these live in `PlatformShell`).

## Workspace resolution contract

Every service module MUST provide `resolveWorkspace(user) → WorkspaceResolution`. This is the single funnel through which a service's per-user state enters the platform. It is the **only** sanctioned place where service-specific user proxies (e.g. FB's `org_id`) are allowed.

```ts
interface WorkspaceResolution {
  state: WorkspaceState         // none | pending_invite | initializing | ready | suspended | billing_blocked
  workspaceId?: string
  reason?: string               // human-readable, shown in non-ready states
  rbac?: { role: string; permissions: string[] }  // for future RBAC-aware UI
}
```

When PR 2 lands the real `/api/platform/services` endpoint, the backend populates this shape directly and `resolveWorkspace` becomes a no-op pass-through. The contract does not change.

## Anti-pattern — Platform god object

The platform layer must stay thin. It MUST NOT acquire ownership of:

- Billing logic
- Browser session management
- AI agent state
- Job queues / workers
- Notification delivery logic (the platform provides a slot; modules emit into it)
- Analytics computation
- Permission resolution (the platform consults the resolver; it does not compute roles)

The platform layer's only responsibilities are:

1. **Orchestration** — mount the right view at the right URL.
2. **Routing** — URL paths, redirects, auth gates.
3. **Hosting contracts** — service registry, `ServiceContext`, `ServiceBoundary`.

If `PlatformShell.tsx` starts to import billing logic, browser session managers, or workflow orchestration, the monolith has returned under a new name. Move it back into the service module (or a new dedicated module).

## Resolution pipeline

Every value rendered by the UI flows through this pipeline. There is no other path. New code that bypasses a stage of this pipeline is rejected on review.

```
┌─────────────────────────────────────────────────────────────────┐
│  Raw storage  (DB rows, JWT claims, in-memory user state)        │
│  └ org_id, plan_tier, role, billing.expires_at, ...              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼  adapters (PR 2: internal/platform/services/adapters/)
┌─────────────────────────────────────────────────────────────────┐
│  Domain entities  (User, Workspace, Membership, ...)              │
│  └ canonical names — see specs/DOMAIN_MODEL.md                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼  per-service resolvers
┌─────────────────────────────────────────────────────────────────┐
│  Service resolution                                              │
│  ├ resolveStatus(user)         → ServiceStatus                   │
│  ├ resolveWorkspace(user)      → WorkspaceResolution             │
│  ├ resolveCapabilities(user)   → ServiceCapabilities             │
│  └ resolveAccess(user)         → AccessResolution                │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼  projection (usePlatformServices)
┌─────────────────────────────────────────────────────────────────┐
│  PlatformService  (cross-boundary contract)                       │
│  └ descriptor + status + workspaceState + access + capabilities  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼  presentation resolver
┌─────────────────────────────────────────────────────────────────┐
│  WorkspacePresentation                                           │
│  └ badge / primaryAction / canEnter / canCreate / ...            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼  render
┌─────────────────────────────────────────────────────────────────┐
│  UI                                                              │
│  └ Cards, buttons, badges. No branching on lifecycle enums.      │
└─────────────────────────────────────────────────────────────────┘
```

**Reverse-direction rule:** information flows DOWN. A UI component never asks for a raw field higher up the pipeline. If the renderer needs a value that isn't in `WorkspacePresentation`, the presentation resolver gets the new field — not the renderer.

## Contract invariants

The service contract shape is load-bearing architecture. These invariants are enforced at registration time by [contractInvariants.ts](services/contractInvariants.ts) — a violation throws at boot, not at runtime.

1. **slug is a non-empty, immutable, lower-kebab/snake string.** Identity, not name.
2. **All IDs are strings.** No numeric IDs cross the contract boundary.
3. **No storage field names.** `org_id`, `plan_tier`, `*_at` etc. must not appear in a contract — contracts are not ORM rows.
4. **descriptor is fully populated.** Every descriptor field is required.
5. **All resolvers are present and callable.** `resolveWorkspace`, `resolveCapabilities`, `resolveAccess`.
6. **Resolvers are total.** They do not throw on a `null` user — they return a well-formed result for every input.

PR 2 backend mirrors these in `internal/platform/services/contracts/` with the equivalent Go assertions.

## Resolver purity rule

Resolvers (`resolveStatus`, `resolveWorkspace`, `resolveCapabilities`, `resolveAccess`, `resolveWorkspacePresentation`) MUST be:

- **Deterministic** — same input ⇒ same output, always.
- **Side-effect free** — no mutation of external state.
- **No network IO** — a resolver never fetches.
- **No DB writes** — a resolver never persists.

A resolver *answers a question*; it does not *do* anything. The moment a resolver performs IO or mutates state, semantic resolution silently becomes hidden business execution — and every guarantee in the resolution pipeline collapses.

If a resolver needs remote data, that data is loaded **upstream** (the adapter layer, the data-loading hook) and passed **in**. The resolver stays pure. This is why `contractInvariants.ts` can safely smoke-call `resolveWorkspace(null)` at boot — a pure resolver has no boot-time cost.

PR 2: backend resolvers are pure functions over already-loaded domain entities. Loading happens in `adapters/`; resolution happens in `resolver/`. Never mixed.

## No reverse inference

Information flows downstream only (see § Resolution pipeline). The reverse direction is forbidden:

| Forbidden reverse inference | Why it's wrong |
|---|---|
| UI state ⇒ business meaning | The UI is a projection of state, not a source of it |
| route params ⇒ authorization | A URL is user-controlled input, never proof of access |
| workspace existence ⇒ usability | Existence is `workspaceState`; usability is `access` + `status` |
| presence of a field ⇒ permission to use it | See § Capability vs Access vs Permission |

A component that reads `params.workspaceId` and assumes the user may therefore enter that workspace has performed reverse inference. The correct path: `params.workspaceId` → resolver → `WorkspaceResolution` + `AccessResolution` → presentation → render decision.

Every semantic conclusion is *resolved downstream*, never *inferred upstream*.

## Registry authority

The current frontend registry is a **bootstrap layer** — it exists so PR 1 could ship before the backend registry. Post-PR 2, the **backend service registry is the authority**: services are known by the backend and delivered to the frontend via `GET /api/platform/services`.

End state: the FE does not *infer* which services exist from which modules are imported — it *receives* the service list from the backend. FE module registration becomes "here is the code that renders service X *if the backend says X exists*", not "service X exists because I imported it".

Do not design FE code that assumes the FE registry is the source of truth. It is a temporary bootstrap.

## Service identity & versioning

- **`slug` is permanent and immutable.** It is the service's identity, not its name. A service is never renamed. A marketing/UX rename changes `descriptor.publicLabel` — never the slug. Renaming a slug is a delete + create, not an edit. Code, URLs, analytics, and stored references all depend on slug stability.
- **`descriptor.version` is the contract version.** Bump it when the service's contract shape changes in a breaking way. The PR 2 `/api/platform/services` response carries a top-level `contractVersion` so stale clients (mobile apps, the Chrome extension, cached metadata) can negotiate or refuse gracefully.

## Capability vs Access vs Permission

Three distinct concepts. Conflating any two is a bug.

| Concept | Question | Where it lives |
|---|---|---|
| **Capability** | "Does the system support this feature for this user's setup?" | `ServiceCapabilities` — `resolveCapabilities(user)` |
| **Access** | "May the user enter / operate the service right now?" | `ServiceAccess` — `resolveAccess(user)` |
| **Permission** | "Within the workspace, what may this membership do?" | `WorkspaceRBAC` — RBAC inside `Membership` |

`capabilities.browserAutomation === true` means *the service supports browser automation*. It does NOT mean the current user may run it. A billing-blocked user still has `browserAutomation: true` as a capability — but `access === 'billing_blocked'` stops them. A read-only member has the capability and access, but lacks the permission.

When gating a UI control: ask which of the three you actually mean. If you reach for `capabilities` to answer "is the user allowed?", you have the wrong concept — use `access` or `permission`.

## State axes — orthogonal, not collapsed

The state of a service for a user is decomposed across **three** axes. Each axis answers one question. No axis encodes another's concern.

| Axis | Type | Question |
|---|---|---|
| `ServiceStatus` | `available \| unavailable \| suspended` | Is the service offered at all? |
| `WorkspaceState` | `none \| initializing \| ready \| suspended` | Does the workspace exist and is it operational? |
| `ServiceAccess` | `granted \| invite_required \| billing_blocked \| region_locked \| admin_blocked` | May the user actually enter? |

These compose at the presentation layer. Example: a `ready` workspace with `billing_blocked` access → renderer shows "billing issue" with a `canRetry` action, not "open workspace".

Collapsing two axes (e.g. putting `billing_blocked` inside `WorkspaceState`) is a bug — it forces every consumer to re-derive the distinction. Keep axes independent.

## No implicit business meaning

Every semantic question passes through a resolution layer. The following implicit inferences are **rejected** on review:

| Rejected inference | Use instead |
|---|---|
| `user.org_id > 0` ⇒ workspace initialised | `service.workspaceState === 'ready'` |
| `service.workspaceId` exists ⇒ workspace usable | `service.workspaceState === 'ready'` |
| `service.status === 'available'` ⇒ workspace accessible | `resolveWorkspacePresentation(service).canEnter` |
| `permissions.length > 0` ⇒ user authorised | A resolver field for that specific authorisation question |
| `subscription.expiresAt > now` ⇒ feature enabled | `service.capabilities.<flag>` |
| `viewer.role === 'admin'` ⇒ can do X | `membership.permissions.includes('X')` |

If you need to ask a semantic question, **extend the resolver** — don't infer from a related field.

The resolution layers in this codebase:

- `service.resolveStatus(user)` → `ServiceStatus` (service-level availability)
- `service.resolveWorkspace(user)` → `WorkspaceResolution` (per-user workspace lifecycle)
- `service.resolveCapabilities(user)` → `ServiceCapabilities` (per-user feature gating)
- `resolveWorkspacePresentation(service, lang)` → `WorkspacePresentation` (UX-level mapping; badge/CTA/can*)
- (PR 2) backend `GET /api/platform/services` returns the resolved shape

When a renderer or handler is about to switch on an enum or check truthy-ness for semantic meaning, the right move is to **add a resolver field** — not another branch.

For broader project-wide naming + ownership rules, see [specs/DOMAIN_MODEL.md](../../../specs/DOMAIN_MODEL.md). The Domain Model is the ubiquitous-language contract; this file is the frontend platform's enforcement layer.

## Boundary audit checklist

Use this before merging any PR that touches `frontend/src/platform/`, `frontend/src/modules/*`, or `internal/platform/` (post-PR 2). Each checkbox is a potential block.

**Resolution & inference:**

- [ ] Does this introduce implicit business meaning (e.g. `value > 0` ⇒ semantic state)?
- [ ] Does any UI infer backend semantics directly (e.g. switching on a raw enum)?
- [ ] Does this bypass the resolution pipeline (skip a layer, read upstream)?

**Ownership:**

- [ ] Does the platform layer own service-specific logic (FB knowledge, Taobao-specific code in `frontend/src/platform/`)?
- [ ] Does a service module mutate platform routing or shell state?
- [ ] Does a new file under `frontend/src/platform/` contain the word "Facebook"? (If yes, it belongs in `frontend/src/modules/autoflow/`.)

**Contracts:**

- [ ] Does any new HTTP/JSON shape expose DB row structure (ORM leak)?
- [ ] Does any new ID flow as a number across a boundary? IDs MUST be strings with canonical prefix (see DOMAIN_MODEL.md).
- [ ] Does any new field use a forbidden synonym (org_id, tenant, account) in NEW code?

**Determinism:**

- [ ] Does the change rely on Map iteration order, insertion order, or other implicit ordering?
- [ ] Are any new lifecycle states orthogonal to existing axes? (If you're adding a value to one enum that semantically belongs to another, you have an axis collapse — split it.)

**Vocabulary:**

- [ ] Does any user-facing copy say "onboarding" for service workspace creation? (Reject — use "Create X Workspace".)
- [ ] Does any new comment/log/commit message use a deprecated synonym?

If any answer is unclear, link the relevant section of this file or [specs/DOMAIN_MODEL.md](../../../specs/DOMAIN_MODEL.md) in the PR description and ask reviewer to confirm.

## Adding a new service

1. Create `frontend/src/modules/<slug>/` with the module's components.
2. Export a `<slug>ServiceModule: ServiceModule` from `frontend/src/modules/<slug>/service.ts`.
3. Add one line to `frontend/src/platform/services/bootstrap.ts`:
   ```ts
   registerService(taobaoServiceModule)
   ```
4. Done. The platform router automatically renders `/services/taobao/...` routes from the contract.

No platform code changes. No route file changes. No top nav changes. If you needed to change anything in the platform layer to add a service, the contract is wrong — fix the contract instead.

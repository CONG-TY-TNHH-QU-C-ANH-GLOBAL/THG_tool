# Module Boundaries

**Status:** OFFICIAL STANDARD. **Companion of** `ARCHITECTURE_STANDARD.md`.
Each module below lists: responsibility, allowed imports, forbidden imports,
"belongs here" and "must NOT belong here" examples. Boundaries are enforced
(warn-only for now) by `scripts/check_import_boundaries.sh`.

**The one rule:** imports point downward only — Drivers → Application/Workflows →
Domain → Infrastructure. Pure modules (`ai`, `fburl`) import only `models` + stdlib.
Same-layer service modules never import each other.

Legend for "current code": ✅ exists and matches · ◐ exists, partial · ○ aspirational
(name reserved; code not yet extracted — rule documented, enforced after the move).

---

## platform  ✅◐

- **Responsibility:** service registry (`internal/platform/services`), workspace/org
  shell, tenancy root (`users`, `organizations`), composition wiring of the service
  list. The neutral shell every service plugs into.
- **Allowed imports:** `models`, `store` (users/org accessors), stdlib, service
  *contracts* (resolver interfaces it hosts).
- **Forbidden imports:** any business service (`services/facebook`, `internal/ai`
  comment/generators, `internal/jobhandlers`, `internal/leadingest`), `drivers/*`.
  The platform hosts services; it must not know their internals.
- **Belongs here:** `Registry.Register(resolver)`, service status resolution,
  org/workspace lifecycle, tenant root tables.
- **Must NOT belong here:** "if service == facebook then crawl" logic; comment
  generation; Facebook URL parsing.

## drivers/copilot  ◐

- **Responsibility:** inbound adapter. Natural-language prompt → normalized intent →
  an **application command** (e.g. `comment_single_post`, `crawl_group`). Routing,
  typo/multilingual NLU, ask-back.
- **Allowed imports:** `internal/ai` intent layer (`intent_*.go`), `internal/fburl`
  (URL trust), the **application command interface** it invokes (a port), `models`.
- **Forbidden imports:** DB repositories directly (`internal/store/<domain>`),
  `internal/server`, `internal/store/outbound`, connector internals. The driver
  hands a command to the application layer; it does not queue or claim outbound itself.
- **Belongs here:** `deterministicFacebookAction`, `promptIsDirectPostComment`,
  intent lexicons, RouteDecision observability.
- **Must NOT belong here:** `db.QueueOutboundForOrg(...)`, `db.Leads().GetLeadByPostRef`,
  readiness/coverage gate logic, browser session handling.
- **Current gap:** `internal/ai/agent.go` holds `*store.Store` and the legacy
  `ActionHandler` reaches store/outbound directly. Tracked in `CURRENT_CODE_AUDIT.md`;
  the boundary script WARNs on it. Target: copilot emits a command to an injected
  application port (see `PORTS_AND_ADAPTERS.md`).

## services/facebook  ◐

- **Responsibility:** Facebook *workflows* (application layer for the FB vertical):
  crawl group/post, import post, plan comment/inbox, post to group/profile. Owns the
  sequencing and the FB-specific gates; delegates execution to outbound + connectors,
  intelligence to `ai`, URL trust to `fburl`.
- **Allowed imports:** `ai`, `fburl`, `outbound` (via its consumer-owned port),
  `connectors`/`identities` (via ports), `leads`/`crawl` domain accessors, `events`
  (publish), `models`.
- **Forbidden imports:** `drivers/copilot` (a service must not depend on the driver
  that calls it), other service modules (`services/taobao`, `services/1688`),
  `internal/server` transport.
- **Belongs here:** `queueLeadOutreach` orchestration, `commentSinglePost`,
  single-post import continuation (workflow/process manager), readiness gate wiring.
- **Must NOT belong here:** raw `execution_attempts` writes (coordination owns them);
  Telegram rendering; HTTP request parsing; Taobao logic.

## services/taobao  ○   ·   services/1688  ○

- **Responsibility:** future sourcing verticals. Same shape as `services/facebook`:
  workflows on shared outbound/connectors/events/ai primitives.
- **Allowed imports:** the shared primitives via ports, `models`. **Forbidden:**
  `services/facebook`, `services/1688`/`services/taobao` (siblings), `drivers/*`.
- **Belongs here:** Taobao/1688 product sourcing, price extraction workflows.
- **Must NOT belong here:** Facebook selectors, FB URL trust, anything FB-specific.
- **Today:** only resolver stubs in `internal/platform/services/resolver`. The rule
  is reserved; nothing to enforce until the module is extracted.

## ai  ✅

- **Responsibility:** PURE intelligence. Classify leads, generate/repair comment &
  inbox copy, score, decide intent shape. Deterministic, IO-free, platform-neutral.
- **Allowed imports:** `internal/models` + stdlib **only**.
- **Forbidden imports:** `store`, `server`, `connector`/`browsergateway`, `outbound`,
  `jobs`, any `services/*` workflow, `platform` workflow. (The Copilot *driver* under
  `internal/ai/agent*.go` is a separate logical module — see drivers/copilot — and
  is held to the driver rules, not the pure-ai rules.)
- **Belongs here:** `internal/ai/comment` decision/repair, message generation prompts,
  classifier schemas.
- **Must NOT belong here:** DB reads, HTTP, Facebook selectors, queueing outbound,
  reading cookies/sessions. Missing grounding → return typed `knowledge_gap`, never
  invent a fact.

## knowledge  ◐

- **Responsibility:** KnowledgeOS assets, sources, feedback, retrieval; the grounding
  substrate every concrete outbound claim must cite.
- **Allowed imports:** `models`, its own store domain (`internal/store/knowledge`),
  `ai` (for embedding/classification via ports), stdlib.
- **Forbidden imports:** `outbound`, `services/*` workflows, `server`, `connectors`.
- **Belongs here:** asset CRUD, retrieval/soak, knowledge events.
- **Must NOT belong here:** comment queueing, Facebook crawl, connector state.

## brand  ◐

- **Responsibility:** brand/company identity, contact profiles, personas — the
  verified facts (company identity, CTA assets) outbound copy is grounded by.
- **Allowed imports:** `models`, its store domain, stdlib.
- **Forbidden imports:** `services/*`, `outbound`, `server`, `connectors`.
- **Belongs here:** `staff_contact_profiles`, persona/brand identity records.
- **Must NOT belong here:** message generation (that's `ai`), queueing.

## outbound  ◐

- **Responsibility:** outbound coordination spine — queue, dedup, claim (CAS/lease),
  transition, finalize, policy, append-only ledger + execution_attempts. Domain-
  agnostic: it coordinates *actions*, not Facebook.
- **Allowed imports:** `models`, `coordination` ledger types (write its entries),
  `events` (publish transitions), `store/dbutil`, stdlib.
- **Forbidden imports:** `services/facebook` (or any service), `drivers/copilot`,
  `fburl`, `internal/jobhandlers`, `internal/server`. Outbound must stay vertical-
  neutral so Taobao reuses it unchanged.
- **Belongs here:** `Queue`, `ClaimPlanned*`, lease/transition, `action_policies`
  evaluation, execution_attempts append.
- **Must NOT belong here:** "build a Facebook comment URL", FB selectors, NL routing.
- **Current state:** `internal/store/outbound` is clean (imports models + runtime/
  events + dbutil). `cmd/scraper/outbound_actions.go` (`queueLeadOutreach`) is the
  application-side orchestrator and is FB-adjacent today; target is to split the
  vertical-neutral core from the FB-specific target-URL resolution.

## events  ◐

- **Responsibility:** durable event bus + transactional outbox + relay. The single
  source of truth for critical cross-module events.
- **Allowed imports:** `models`, `store` (outbox table), stdlib.
- **Forbidden imports:** any `services/*`, `drivers/*`, business domain internals.
  Events carry data, not behavior.
- **Belongs here:** outbox table writer, relay loop, event envelope, idempotency keys.
- **Must NOT belong here:** "when FacebookLeadCreated, comment it" — that is a
  process manager in `services/facebook`, subscribing to the event, not in `events`.
- **Today:** `internal/events/bus.go` is an **in-memory** SSE bus (drops on slow
  consumer) and `runtime_events` exists as an audit table. Neither is yet a durable
  transactional outbox — see `TRANSACTIONAL_OUTBOX.md` and roadmap Phase E.

## notifications  ◐

- **Responsibility:** deliver notifications (per-org Telegram channel, in-app bell)
  from durable events. A *sink*, not a source of business logic.
- **Allowed imports:** `models`, `telegram` client, its store domain, `events`
  (subscribe), stdlib.
- **Forbidden imports:** `services/facebook` lead/comment logic, `outbound` internals,
  `connectors`. Notifications must NOT own Facebook lead logic — it renders a
  `LeadCreated`/`CommentPosted` event into a message, nothing more.
- **Belongs here:** `control.NotifyLead`, channel routing, in-app bell rows.
- **Must NOT belong here:** deciding whether a lead qualifies, queueing a comment,
  reading FB sessions.

---

## Cross-cutting forbidden patterns (all modules)

1. **No global contracts god-package.** There is no `internal/contracts` dumping
   ground. Each interface lives with its consumer (`PORTS_AND_ADAPTERS.md`).
2. **No raw `execution_attempts` / `action_ledger` writes** outside
   `internal/store/coordination`.
3. **No cross-module DB write without the owner's API** (`DATABASE_OWNERSHIP.md`).
4. **No critical cross-module side effect via composition-root callback** — use the
   transactional outbox + a process manager.
5. **No secrets** (cookies/tokens/session) in logs, event payloads, or SQL results.
6. **Service modules never import siblings.** Facebook ⟂ Taobao ⟂ 1688.

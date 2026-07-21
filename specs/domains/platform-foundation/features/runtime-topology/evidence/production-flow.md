# THG AutoFlow — Production Flow Reference

> Living document. Update this whenever you change a load-bearing pipe in
> production. Other agents and engineers will read this before touching
> sensitive paths (auth, outbound, browser sessions, AI prompts).
>
> Last refactor: **2026-05-03** — security wall + refactor A–E.

This file complements (does not replace):

- [`AGENTS.md`](../../../../../../AGENTS.md) — short operating instructions per topic.
- [`specs/BROWSER_GATEWAY_AND_FACEBOOK_AUTOMATION_VISION.md`](../../browser-connector/decisions/browser-gateway-vision.md)
  — browser provider direction and Chrome Extension production path.
- [`facebook-sales-intelligence roadmap.md`](../../../../facebook-sales-intelligence/roadmap.md)
  — long-form product / feature direction.
- [`openspec/root-architecture.md`](../../../../../../openspec/root-architecture.md)
  — system boundaries.

## 1. North star (read first)

> **AI Facebook Sales Intelligence Workspace, per-business.**

The product is **not** a Facebook scraper, **not** a spammer, **not** an
internally-hardcoded tool. Every vertical (HR, POD, sales, support
monitoring, etc.) is a *blueprint* over a shared set of primitives.

User-facing entry points (dashboard chat / Telegram / API) must accept a
**single open prompt** and let the agent orchestration layer route the
intent to the correct skill. Outbound automation defaults to
**approval-required** unless the org/campaign has explicitly opted in.

## 2. Topology

```
┌────────────────────┐     ┌────────────────────┐
│ Frontend (Next.js) │ ──► │  Go API (Gofiber)  │
│  /frontend         │     │  cmd/scraper       │
└────────────────────┘     └─────────┬──────────┘
                                     │
                          ┌──────────┼──────────┐
                          ▼          ▼          ▼
                   ┌──────────┐ ┌─────────┐ ┌──────────┐
                   │  SQLite  │ │ Workers │ │ Telegram │
                   │  WAL on  │ │  go     │ │  bot     │
                   └────┬─────┘ │  routs  │ └──────────┘
                        │       └────┬────┘
                        │            │
                        │   ┌────────┴────────────┐
                        │   ▼                     ▼
                        │ ┌──────────┐    ┌──────────────────┐
                        │ │ Workspace│    │  THG Local       │
                        │ │ Manager  │    │  Runtime / Conn. │
                        │ │ (Docker) │    │  (user laptop)   │
                        │ └────┬─────┘    └────────┬─────────┘
                        │      │                   │
                        │      ▼                   ▼
                        │   ┌──────────────────────────────┐
                        └──►│ Per-account Chrome (CDP+VNC) │
                            └──────────────────────────────┘
```

Two browser execution modes coexist:

1. **Workspace mode** — server starts a Docker container with Chrome
   per Facebook account, dashboard streams VNC + sends CDP via
   `/ws/screen/:id`.
2. **Chrome Extension connector mode** — user installs THG Chrome
   Extension in the trusted Chrome profile, pairs with the dashboard
   via a one-time pairing code, reports presence + screenshot frames
   to the API, polls prompt-scoped crawl commands, and executes
   approved outbox actions.

Both modes converge on the same `browser_sessions` table and the same
identity-sync helper (see §5).

## 3. Boot sequence (`cmd/scraper/main.go`)

1. `config.Load()` — read env vars, defaults.
2. `cfg.MustValidateProductionSecrets()` — `log.Fatal` if
   `APP_ENV=production` and `JWT_SECRET` / `ENCRYPTION_KEY` missing.
   This is the single guard against booting prod with cookies stored
   plaintext or auth disabled.
3. `store.New(cfg.DBPath)` — open SQLite, run migrations.
4. `db.SetEncryptionKey(cfg.EncryptionKey)` — required for
   AES-256-GCM on `accounts.cookies_json`.
5. Bootstrap admin / superadmin from `ADMIN_*` / `SUPERADMIN_*` env.
6. `db.ResetOrphanedOutbounds()` — clear stale `approved` rows from a
   prior crash so we don't re-send.
7. `jobs.NewStore` (scheduler_jobs) + `store.NewAppStore` (app_tasks,
   browser_sessions, …).
8. `workspace.NewManager`, `workspace.NewPortRegistry` — reconcile
   running Docker containers so a server restart doesn't orphan a
   live login.
9. `workspace.NewCircuitBreaker` + `RestartController` +
   `HealthChecker` (15 s tick).
10. `session.NewRegistry`, `ai.NewPriceExtractor`, `ai.NewAgent`,
    `telegram` bot.

## 4. Multi-tenant boundary (Phase 1)

**Rule:** every tenant-facing record must be filtered by `org_id`.
Non-superadmin users with `org_id == 0` must not access tenant APIs.

The check now lives at the **data layer** so handlers can't accidentally
forget it.

| Helper | When to use |
|---|---|
| `store.GetAccountForOrg(id, orgID)` | All tenant-facing handlers. Returns `nil, sql.ErrNoRows` if `acc.OrgID != orgID` (does not leak existence cross-tenant). |
| `store.GetAccount(id)` | Internal/worker code that has already proved org ownership upstream (e.g. agent token-bound handlers). Treat as a sharp tool. |
| `(s *Server) requireAccountForOrg(c, accID, orgID)` | HTTP fiber handlers. Writes the 404 response itself; caller returns the error untouched. |
| `(s *Server) requireAccountForOrgWS(orgID, role, accID)` | WebSocket handlers (VNC, screen proxy). Honours `IsPlatformUser` for founder/superadmin. |
| `(s *Server) rejectIfFacebookProfileMismatch(c, ctx, acc, fbUID, orgID)` | Before persisting any Facebook identity update. Writes 409 + `local_error` session row on mismatch. |

Platform users (founder, superadmin) bypass the check via
`models.IsPlatformUser(orgID, role)` — they need cross-tenant
observability for support.

## 5. Connector / identity sync pipeline (Phase 1, refactor C)

The same shape arrives at three endpoints from THG Chrome Extension:

- `POST /api/agent/heartbeat` — presence ping every ~5 s.
- `POST /api/agent/chrome-status` — explicit handshake.
- `POST /api/agent/screenshot` — per-frame stream payload.

All three convert their request body into a
`server.connectorIdentitySnapshot`, run
`s.applyConnectorIdentity(ctx, snap)`, and stop. The pipeline:

```
applyConnectorIdentity(snap):
  1. status = LocalSessionStatusFromStream(snap.StreamStatus)
  2. AppStore.RecordLocalSession(...)             // upsert row
  3. if logged_in:
       SetAccountFacebookIdentity(...)            // FBUserID locked
       UpdateAccountStatus(active)
     else if LocalFacebookNotReady:
       SetBrowserLoggedInState(false)             // clear cached flag
```

**Do not duplicate this logic.** New connector endpoints should call
the helper instead of mutating browser_sessions / accounts directly.

Before calling the helper, the handler must:

1. `requireAccountForOrg` (or `GetAccountForOrg` for non-fiber).
2. `rejectIfFacebookProfileMismatch` if the connector reports a
   `fb_user_id` that disagrees with the account slot.

## 6. Session lifecycle (typed enum)

Single source of truth: [`internal/store/session_status.go`](../../../../../../internal/store/session_status.go).

```
local_starting   ──► local_active ──► local_ready
                                  ╲          │
                                   ╲ login   │ checkpoint
                                    ╲▼       ▼
                                  local_login_required / local_human_required
                                    │
                                    ▼
                                 local_error / local_terminated
```

Stream → typed status mapping is `LocalSessionStatusFromStream`.
Use the typed enum, not raw strings — typo-proof and grep-friendly.

For Docker workspace mode the analogous statuses are
`initializing → display_ready → ready` then `idle | error |
checkpoint | terminated`.

## 7. Outbound flow (Phase 2 — DONE 2026-05-04)

Tables: `outbound_messages`, `threads`, `thread_messages`.

States: `draft → approved → sent | failed | rejected`.

### 7.1 AI / agent / Telegram path — `QueueOutboundForOrg`

This is the canonical write path. **Always use it from any code that
runs LLM-derived intent.** It performs all three guards atomically:

```go
result, err := db.QueueOutboundForOrg(&models.OutboundMessage{
    OrgID:      orgID,
    Type:       "comment",     // comment | inbox | group_post
    Platform:   models.PlatformFacebook,
    AccountID:  accountID,
    TargetURL:  targetURL,
    TargetName: lead.Author,
    Content:    aiGenerated,
    AIModel:    "agent",
}, requestedAuto, 24*time.Hour)
```

What the helper guarantees:

1. **Dedup + cooldown** (transactional `CanQueueOutboundForOrg` —
   same SELECT and INSERT see one snapshot under SQLite WAL).
2. **Store-layer approval policy** — `requestedAuto=true` is only
   honoured when `org:{id}:outbound_mode == "auto"` in user_context.
   Otherwise the row is downgraded to `draft`. AI tools cannot flip
   this flag; the `set_context` server handler explicitly rejects
   keys `outbound_mode` / `auto_comment_mode`.
3. **Partial UNIQUE index** `idx_outbound_active_target` on
   `(org_id, type, target_url) WHERE status IN ('draft','approved')`
   is the last-line guard against two transactions racing past the
   application-level guard.

`OutboundQueueResult.Decision.Reason` is one of:
`ok | duplicate_outbound_target | duplicate_outbound_target_race |
outbound_cooldown_active | conversation_closed |
awaiting_reply_cooldown | lead_replied | missing_target_url`.
Surface the reason to operators instead of treating it as an error.

### 7.2 Manual admin path — `InsertOutboundMessage`

Direct insert is reserved for admin actions where the operator has
chosen the status explicitly (dashboard "approve & send", restoring
a draft). It does NOT consult the auto-mode flag — the admin is
the policy.

### 7.3 Send path

1. Approval → `UpdateOutboundStatusForOrg(approved)` →
   `wsHub.NotifyOutboxReady`.
2. Local agent / worker pulls via `GET /api/agent/outbox`, sends, then
   `POST /api/agent/outbox/:id/sent` or `…/failed`.

### 7.4 Auto-mode opt-in (admin only)

To enable auto-execute for an org:

```sql
INSERT OR REPLACE INTO user_context (key, value, updated_at)
VALUES ('org:{ID}:outbound_mode', 'auto', CURRENT_TIMESTAMP);
```

Or via the Settings page in the dashboard (admin role required).
This is an explicit per-org commitment; do not enable it on behalf
of the user from any LLM-driven path.

## 8. Legacy local-job lease (Retired 2026-05-05)

The old `/api/agent/jobs/next` connector job queue was removed with the
previous connector implementation. Prompt-scoped crawl work now goes through
`connector_commands`; outbound execution goes through
`/api/connectors/outbox`; recurring crawl intent goes through
`scheduler_jobs` / `app_tasks`.

Do not add new automation to the old `jobs` table. It may remain in
existing SQLite databases for migration compatibility, but it is no
longer an execution path.

## 8b. Browser lifecycle hardening (Phase 3 — DONE 2026-05-04)

The browser layer added four production guards on top of the pre-existing
container manager:

- **Cross-process profile lock** — `internal/workspace/profile_lock.go`
  takes an exclusive `O_CREATE|O_EXCL` claim on
  `<profile>/.thg-profile.lock` before docker-running a container. A
  stale stamp older than `profileLockTTL = 30 m` is considered abandoned
  (post-crash) and gets overwritten. Manager.Stop / StopAll release the
  lock; a failed Start releases via deferred cleanup.
- **Per-account restart debounce** — `RestartController` keeps a map of
  in-flight + recently-finished restarts per accountID and refuses
  re-entry while a goroutine is mid-restart, plus a 30 s cooldown after
  it returns. The HealthChecker is single-threaded today but external
  callers (watchdog, manual /restart) cannot race past this guard.
- **Checkpoint → ready CDP verifier** — `session.CheckpointVerifier` is
  wired in the API server (`workspaceCheckpointVerifier`) and runs
  `facebookSessionSnapshotFromInstance` before
  `CheckpointManager.ResolveCheckpoint` flips the row. If Chrome is
  still parked on a verification page the call returns
  `*ErrCheckpointStillActive` and the HTTP handler returns 409 with
  code `CHECKPOINT_STILL_ACTIVE` — the operator stays on VNC.
- **Frontend org-switch cleanup** — `BrowserView` clears `selectedId`,
  `sessionInfo`, and `localScreen` on `orgId` change so the embedded
  `VncCanvas` unmounts and its WebSocket closes before the workspace
  list re-fetches the new org. Server-side guards still enforce the
  org boundary; this is the visual-isolation half.

## 9. Connector ownership (Phase 1, refactor E)

Helper: `store.ConnectorOwnsAccountStream(orgID, agentID, accountID)`.

Required before persisting any work attributed to an `accountID` from
a connector:

- crawl results (`POST /api/connectors/crawl-result`)
- queued input commands
- screenshot ownership

Ownership rule: agent's most recent `connector_screenshots` row OR
agent currently online with assignment (`AssignedAccountID == 0` or
== accountID).

## 10. Input bounds (Phase 1, [`input_limits.go`](../../../../../../internal/server/input_limits.go))

Every connector-supplied string passes through `clampPresenceFields`
before reaching `UpdateAgentPresence`. Limits picked to be generous
for legit traffic but block flooding:

| Field | Max runes |
|---|---:|
| Hostname / OS / Kind / Transport | 64–128 |
| Version / StreamStatus | 64 |
| FBUserID | 64 |
| FBUsername | 80 |
| FBDisplayName / Email | 255–320 |
| FBProfileURL | 512 |
| CurrentURL | 2048 |
| CapabilitiesJSON / ChromeError | 4096 |
| Screenshot data URL | 6 MB (handler check) |

Truncation is rune-aware so multi-byte Vietnamese names don't get
chopped mid-codepoint.

## 11. AI prompt safety (Phase 1, classifier)

User-controlled Facebook content (post body, group name, author name)
is **untrusted**. The classifier wraps it in a JSON envelope inside
explicit `BEGIN USER_DATA … END USER_DATA` markers, with a system
instruction that everything inside USER_DATA is data, not commands.

`sanitizeForPrompt(value, maxRunes)`:

- strips control chars (incl. zero-width attacks below 0x20)
- replaces tabs/newlines with spaces
- truncates to `maxRunes`

Apply the same envelope pattern to any new LLM prompt that mixes
user-controlled data with instructions (msggen, planner, agent
loop, …). Phase 6 design must enforce this on the open-prompt agent.

## 11b. Auth & WebSocket hardening (Phase 4 — DONE 2026-05-04)

Three hardening passes landed together because production has no
real data yet — no migration window needed and the same SPA fetch
loop sees all three at once.

### 11b.1 CDP input allowlist (Phase 4a)

`internal/server/screen_proxy.go` only accepts a typed `inputEvent`
envelope from the FE (`mouse | wheel | key`). On top of that envelope
the server now whitelists the actual CDP method values:

```go
allowedMouseAction(a)  // mousePressed | mouseReleased | mouseMoved
allowedMouseButton(b)  // none | left | middle | right | back | forward
allowedKeyAction(a)    // keyDown | keyUp | rawKeyDown | char
```

Any FE message outside the envelope (or carrying an out-of-set
action) is silently dropped. This blocks `Action="Runtime.evaluate"`
or `Action="Debugger.enable"` from being smuggled past the type
switch. The FE never speaks raw CDP; the server is the only client
of `/json/version` and the CDP WebSocket.

### 11b.2 Access-token cookie (Phase 4b)

Login / refresh / signup / Google OAuth / org-create / invite-accept
all set two cookies in addition to the JSON body:

| Cookie | HttpOnly | Path | TTL | Purpose |
|---|---|---|---|---|
| `access_token` | yes | `/` | access TTL (short) | JWT carrying `user_id`/`org_id`/`role` |
| `autoflow_session` | no | `/` | **refresh TTL (long)** | SPA-readable presence flag (value `"1"`) |
| `refresh_token` | yes | `/api/auth` | refresh TTL | rotation token, unchanged |
| `g_at` (Google OAuth handoff) | yes | `/api/auth` | 60 s | internal exchange to `/api/auth/google/token` only |

`SameSite=Strict` on all four. `Secure` flips on for HTTPS via
`secureCookie(c)` (existing helper). Logout calls `clearAuthCookies`
which expires both new cookies in addition to the refresh cookie.

**Why the presence cookie outlives the access cookie:** the SPA reads
`autoflow_session` on boot to decide whether to attempt session
restore. If both cookies expired together (access TTL), reopening
the tab after the access token died — but with the refresh cookie
still alive — would short-circuit to login screen. Phase 4 follow-up
fix: presence cookie is now expired only when the refresh cookie
expires, and `restoreSession()` also tries one explicit `/auth/refresh`
before concluding "logged out" if the presence flag is missing.

The JSON body still echoes `access_token` so non-browser clients
(Telegram bot, CLI smoke tests, future server-to-server) keep
working. Browser code must not persist that value to localStorage —
the SPA reads `autoflow_session` to know "I am logged in" and lets
the cookie carry the JWT. `auth.RequireAuth → extractToken` already
preferred Authorization, falling through to the `access_token`
cookie; that order is unchanged.

### 11b.3 WebSocket cookie auth (Phase 4c)

`wsJWTAuth` in `internal/server/api.go` now resolves the WS upgrade
token in this order:

1. `access_token` cookie (browsers send cookies on WS upgrade)
2. `Authorization: Bearer …` header (programmatic clients)
3. `?token=…` query param (legacy fallback only)

`/api/logs/stream` SSE follows the same precedence (cookie → header
→ query) so the SPA never has to mint URLs containing the JWT.

`VncCanvas.tsx` dropped the `?token=…` URL parameter — the upgrade
now goes through with just the cookie. `refreshToken()` is still
called before the upgrade so a stale cookie doesn't reject the
handshake.

The query-param path stays for one release cycle so older runtimes
have time to migrate. **Sunset switch:** set
`WS_AUTH_ALLOW_QUERY_TOKEN=0` in production to disable the legacy
query-token fallback — once telemetry shows no upgrades arrive with
a query token, flip this and the leak surface is gone.

### 11b.4 SPA boot flow

`useAuthStore.hydrate()` runs on mount in `useAuth.ts`:

1. `authService.hasSessionCookie()` reads `autoflow_session=1`.
2. If present → `GET /api/auth/me` (cookie-authed) → restore user.
3. If 401 → `apiFetch` triggers `/auth/refresh` → retry → if still
   401, the user is treated as logged out and the SPA shows the
   auth screen.

No localStorage is touched on the auth path anymore.

## 12. Security secrets table

| Env var | Required in production? | Used for |
|---|---|---|
| `JWT_SECRET` | YES (fail-fast) | HMAC-SHA256 signing access tokens. |
| `ENCRYPTION_KEY` | YES (fail-fast) | AES-256-GCM on `accounts.cookies_json`. |
| `OPENAI_API_KEY` | Strongly recommended | AI classifier / agent / pricer. Without it, AI features degrade silently. |
| `APP_ENV` | YES, set to `production` | Triggers fail-fast checks above. |
| `WORKSPACE_STOP_ON_SHUTDOWN` | optional | Default off so containers survive API restart. |
| `WORKSPACE_SESSION_WATCHDOG` | optional | Off by default to keep VNC-only login flow safe. |
| `WS_AUTH_ALLOW_QUERY_TOKEN` | optional | Default `1` (allow `?token=…`). Set to `0` to disable legacy query-token fallback once all clients use cookie/Authorization auth. |

## 13. Where features live (quick map)

- **Boot** — `cmd/scraper/main.go`.
- **Routes** — `internal/server/api.go`.
- **Auth** — `internal/auth/`, `internal/server/auth_handlers.go`,
  `internal/server/google_auth.go`.
- **Multi-tenant guards** — `internal/server/account_guard.go`,
  `internal/store/accounts.go`.
- **Identity sync** — `internal/server/identity_sync.go`.
- **Browser containers** — `internal/workspace/`,
  `internal/livesession/`.
- **Chrome Extension connector bridge** — `internal/server/agent_handlers.go`,
  `internal/server/local_connector_handlers.go`,
  `internal/store/agent_tokens.go`, `local-connector-extension/`.
- **Job/task pipeline** — `internal/jobs/`,
  `internal/store/app_store.go`.
- **AI** — `internal/ai/business.go` (profile),
  `internal/ai/classifier.go` (lead classifier — sanitised),
  `internal/ai/universal.go` (comment + inbox generation),
  `internal/ai/agent.go` (function-calling agent),
  `internal/agentloop/` (planner + executor).
- **Outbound execution** — `internal/store/outbound.go`,
  `internal/store/threads.go`, agent outbox endpoints in
  `agent_handlers.go`.
- **Frontend dashboard** — `frontend/src/modules/autoflow/`.

## 14. Open-prompt agent skills (Phase 6 — DONE 2026-05-04)

The dashboard chat box and Telegram bot share **one** entry point —
`Agent.ProcessPromptForOrgWithAccount` — and route every user prompt
through a typed skill registry. No vertical (HR, POD, sales, …) is
hardcoded; every capability is a `skills.Skill` registered at boot.

### Skill registry (`internal/skills/`)

```go
type Skill struct {
    ID             string         // snake_case, stable
    Title, Description string
    Category       skills.SkillCategory  // post|comment|inbox|scrape|care|admin
    Outbound       bool          // true → must use QueueOutboundForOrg
    NeedsAccount   bool
    DefaultEnabled bool
    Parameters     []SkillParam   // typed; rendered as JSON Schema
    Run            SkillRun       // ctx, env, args → SkillResult
}
```

- `Registry.EnabledFor(ctx, db, orgID)` — applies the per-org
  blueprint (`org_skills` table). Empty rows → default blueprint.
- `Registry.Execute(ctx, env, id, args)` — single execution point;
  type-checks params, applies max-rune sanitization (re-using the
  `ai.sanitizeForPrompt` rule), runs the skill, writes one audit
  row in `skill_executions`.
- `OpenAITools(enabled)` — projects skills to the OpenAI function
  calling tool list. The LLM only sees skills the org has actually
  enabled.

### Built-in skills (Phase 6.2 + 6.3)

| ID | Category | Outbound? | Description |
|---|---|---|---|
| `scrape_group` | scrape | no | Cào group/post URL |
| `scrape_comments` | scrape | no | Đọc comments của 1 post |
| `search_groups` | scrape | no | Tìm group/page phù hợp |
| `auto_comment` | comment | yes | Comment 1 post cụ thể |
| `comment_all_leads` | comment | yes | Bulk comment leads |
| `auto_inbox` | inbox | yes | Inbox 1 lead cụ thể |
| `inbox_all_leads` | inbox | yes | Bulk inbox leads |
| `create_job_post` | post | yes | Soạn post lên group |
| `describe_business` | admin | no | Lưu business profile |
| `set_context` | admin | no | Lưu context (private files / data sources) |
| `get_stats` | admin | no | Đọc workspace stats |
| `add_group` | admin | no | Đăng ký group source |
| `classify_leads` | admin | no | Confirm classify chạy inline |
| `scan_fanpage_inbox` ★ | inbox | yes | Quét Messenger fanpage *(scaffold; live exec sau Phase 4)* |
| `care_fanpage` ★ | care | no | Pin / react / scheduled post *(scaffold)* |
| `post_to_profile` ★ | post | yes | Đăng lên timeline cá nhân *(reuse group_post pipeline)* |

### Approval policy (unchanged from Phase 2)

Outbound skills MUST go through `Store.QueueOutboundForOrg` which
respects `Store.IsAutoOutboundEnabledForOrg(orgID)` — only an org that
has explicitly set `org:{id}:outbound_mode = auto` can have rows
inserted as `approved`. The `set_context` server handler rejects
attempts to write `outbound_mode` / `auto_comment_mode`, so AI tools
cannot self-promote.

### Per-org enablement (`org_skills`)

- Zero rows → all `DefaultEnabled` skills are active.
- Admin can enable / disable any skill via:

```
PUT /api/skills/:id/enable    (adminOnly)
PUT /api/skills/:id/disable   (adminOnly)
```

`config` JSON column is reserved for future per-skill caps (rate
limits, defaults). Same admin-only write rule as `outbound_mode`.

### Audit trail (`skill_executions`)

Every prompt → skill → result writes one row. Read via:

```
GET /api/skills              catalog (every skill + enabled flag)
GET /api/admin/skills        unfiltered catalog (admin)
GET /api/skills/executions   recent audit rows for the caller's org
```

A daily-ish prune via `Store.PruneSkillExecutions(maxAge)` keeps the
table from growing unbounded — wire it next to the existing scheduler
maintenance loops.

### Why dashboard chat & Telegram share one path

Both endpoints call into `Agent.ProcessPromptForOrgWithAccount`. The
skill resolver is the only divergence, and it is identical for both
sources — only the `Source` field on the audit row distinguishes
`"dashboard"` from `"telegram"`. This is the "open-prompt = same
protocol everywhere" guarantee.

### Live-execution scaffolding (Phase 6.3 → 6.3b)

`scan_fanpage_inbox` and `care_fanpage` ship as **scaffolds** in
6.3 — they return an audit-only acknowledgement with the requested
parameters. Live Chrome-driving execution lands after the Chrome
Extension has dedicated fanpage inbox/care adapters.

`post_to_profile` uses the shared outbound queue but writes a distinct
`profile_post` type, so profile automation can be audited and executed
without being confused with group posting.

## 15. When to update this file

Update **before** merging a change that:

- Adds or moves a top-level pipe (auth, outbound, identity sync,
  connector ownership, agent loop, browser lifecycle, prompt
  templates, secrets handling).
- Introduces a new helper that other handlers should adopt
  (then add a row to §4 / §6 / §8 etc.).
- Changes a contract in `applyConnectorIdentity`,
  `requireAccountForOrg`, `LocalSessionStatus`, or the outbound
  state machine.

If you only fixed a typo or a UI string, you don't need to touch this
file. But anyone reading this list should be able to onboard in
~30 minutes without spelunking through git log.

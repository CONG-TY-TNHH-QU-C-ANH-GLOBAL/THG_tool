## Context

The codebase already has:
- `organizations` table with `id`, `name`, `domain`, `plan_tier`, `max_accounts` columns.
- `org_id` stamped on `users`, `accounts`, `groups` (via `ALTER TABLE` migrations in `store.go`).
- `auth.Claims` carries `OrgID int64` and `Role string` in every JWT.
- Store-level org scoping on `GetAllGroups(orgID)`, `GetAllAccounts(orgID)`, `GetRecentPosts(..., orgID)`, `GetLeadsFiltered(..., orgID)`.
- `users.role` values are `admin` / `sales` (no superadmin role yet).
- No `api_keys` table; no programmatic access path.
- All browser APIs (`/browser/start`, etc.) ignore `orgID` from the JWT — any logged-in user can start any account's container.

The gap: the data model is 80% there but the API enforcement layer is missing entirely. This change adds the enforcement layer, API keys, and the control plane for provisioning orgs.

## Goals / Non-Goals

**Goals:**
- Fiber middleware `OrgScope` extracts `orgID` from JWT or API key on every request; injects it into `c.Locals("orgID")`. All browser handlers read from context — no handler calls SQL without an `orgID`.
- `POST /browser/start`: validate that `account_id` belongs to caller's `orgID` before scheduling. Return HTTP 403 on mismatch.
- Per-org `max_concurrent_browsers` pulled from `organizations.max_concurrent_browsers` (falling back to plan tier defaults). Scheduler semaphore becomes per-org, not global.
- `api_keys` table: hashed SHA-256 key storage; plaintext shown once; keys carry `{org_id, role}`.
- `/api/v1/control/` endpoints protected by `role=superadmin` middleware. `superadmin` is a `users.role` value (new), set via `SUPERADMIN_EMAIL` at bootstrap.
- `/api/v1/org/` endpoints protected by `role=admin OR superadmin` with org scoping.
- Plan tiers `starter` / `growth` / `enterprise` define default quotas; per-org overrides stored in `organizations`.

**Non-Goals:**
- OAuth2 / OIDC provider — API keys + JWT is sufficient.
- Stripe billing integration — plan tiers are labels, not payment events.
- Row-level security in SQLite (not supported) — enforced at the Go query layer.
- Cross-org resource sharing.
- Per-user API keys (keys are org-scoped, not user-scoped).

## Decisions

### 1. API key stored as SHA-256 hash, validated in constant time

**Decision**: `api_keys.key_hash = hex(SHA256(plaintext_key))`. Validation: hash the incoming bearer token and compare with `bytes.Equal` (constant time). Plaintext generated as 32 random bytes → base64url-encoded (43 chars). Prefix format: `thg_<base64url>` for easy identification in logs.

**Why**: SHA-256 is fast enough (no need for bcrypt for a random high-entropy key) and the `thg_` prefix lets engineers instantly recognize a leaked key in a log file. Constant-time compare prevents timing attacks.

**Alternative considered**: bcrypt — appropriate for passwords (low entropy user input), not for 256-bit random keys. Overhead without benefit.

### 2. Per-org scheduler semaphore via `OrgSemaphoreRegistry`

**Decision**: Replace the single global `chan struct{}` semaphore in `Scheduler` with an `OrgSemaphoreRegistry`: `map[orgID → chan struct{}]` guarded by a `sync.RWMutex`. When a job is submitted, the scheduler looks up (or lazily creates) the semaphore for that `orgID` with capacity `org.max_concurrent_browsers`. When an org's quota changes, the registry is updated on next job submission (no hot-resize needed for the channel — a new channel is created at the new capacity and the old one is drained).

**Why**: A global semaphore cannot express per-org limits. Lazy creation means orgs that never use browsers don't allocate channels. The map is small (one entry per active org).

**Alternative considered**: Database-level counter with `SELECT FOR UPDATE` (not supported by SQLite) or optimistic retry — too complex. In-process map + semaphore is correct for single-node.

### 3. `OrgScope` middleware handles both JWT and API key auth in one pass

**Decision**: Fiber middleware applied to all protected routes:
1. Check `Authorization: Bearer <token>` header. If present and starts with `thg_`: validate as API key (hash, DB lookup, `revoked_at IS NULL`). If starts with `eyJ`: validate as JWT. If cookie `access_token` is set: validate as JWT.
2. On success: inject `orgID`, `role`, `userID`/`keyID` into `c.Locals`.
3. On failure: return HTTP 401.

**Why**: A single middleware keeps auth logic in one place. The `thg_` prefix allows O(1) dispatch between code paths without trying both.

**Alternative considered**: Separate middleware per auth type requiring explicit route-level selection — more flexible but forces every route to declare its auth type; easy to forget the API-key middleware on a new route.

### 4. `organizations.max_concurrent_browsers` overrides plan-tier default; zero means use tier default

**Decision**: Plan tier defaults are Go constants:
```go
var PlanDefaults = map[string]PlanQuotas{
    "starter":    {MaxConcurrentBrowsers: 3,   MaxAccounts: 5,  WarmPoolSize: 1},
    "growth":     {MaxConcurrentBrowsers: 20,  MaxAccounts: 30, WarmPoolSize: 5},
    "enterprise": {MaxConcurrentBrowsers: 100, MaxAccounts: 200, WarmPoolSize: 20},
}
```
`org.EffectiveQuota()` returns the per-org override if non-zero, else the tier default. Superadmin can set any override.

**Why**: Zero-as-unset is idiomatic for SQLite integer columns; avoids a separate nullable column. Superadmin overrides allow one-off enterprise deals without changing code.

### 5. Superadmin bootstrap via `SUPERADMIN_EMAIL` env var

**Decision**: At startup, `store.SeedSuperadmin(email)` runs:
```sql
UPDATE users SET role='superadmin', org_id=0 WHERE email=?
```
If no user with that email exists, a stub entry is created with a random password (must be reset on first login). `org_id=0` is the implicit "platform" org used for all superadmin-scoped queries.

**Why**: No separate superadmin table needed. `org_id=0` already means "all orgs" in existing store queries (`orgID=0` returns all in `GetAllGroups` etc.). The env var bootstrap prevents the need for a manual DB edit after initial deploy.

**Alternative considered**: A `superadmin` flag column — redundant with `role='superadmin'`; avoided.

### 6. Control plane API versioned under `/api/v1/`; existing `/api/` routes left unchanged

**Decision**: New routes live under `/api/v1/control/` and `/api/v1/org/`. Existing routes under `/api/` (e.g., `/api/accounts`, `/api/groups`) remain at their current paths and gain org-scoping middleware but no URL changes.

**Why**: Versioned prefix signals that these are stable contract endpoints (suitable for API key callers). Existing `/api/` routes are dashboard-internal; changing their URLs would break the frontend.

**Migration for browser endpoints**: `/browser/start` stays at its current path but gains org enforcement. Callers using API keys must include `account_id` in the request body (already required since `browser-scheduler` change).

## Risks / Trade-offs

- **Per-org semaphore map grows if org count is large** → Mitigation: map is lazily populated; only orgs with active browser sessions have entries; entries are removed when semaphore is fully drained and org has no running containers (background cleanup goroutine).
- **`ALTER TABLE` migrations on production SQLite can lock briefly** → Mitigation: the `organizations` column additions are already done by prior migrations; new `api_keys` table creation is a simple `CREATE TABLE IF NOT EXISTS` (non-locking).
- **API key leaked in logs** → Mitigation: `thg_` prefix makes them grep-able; keys are logged only on creation (once); request logs must never log the full `Authorization` header (enforced by Fiber's default log format, which logs only the route, not headers).
- **BREAKING change to `POST /browser/start`** → Mitigation: `account_id` was already a required field since `browser-scheduler`; the new enforcement is the org-ownership check, which only affects callers attempting cross-org access (no legitimate caller should be doing this).
- **`org_id=0` collision** → Superadmin users have `org_id=0`; `OrgScope` middleware must never inject `orgID=0` as a scoping value for browser APIs — it must instead bypass org enforcement (superadmin can access all orgs). Enforced by a conditional in `OrgScope`.

## Migration Plan

1. Add `api_keys` table migration in `store.go`.
2. Add `max_concurrent_browsers`, `warm_pool_size` columns to `organizations` via `ALTER TABLE IF NOT EXISTS` (idempotent).
3. Add `superadmin` to the set of valid `users.role` values; add `SUPERADMIN_EMAIL` bootstrap in startup sequence.
4. Implement `OrgScope` middleware; wire to all existing protected routes.
5. Implement `OrgSemaphoreRegistry`; swap into `Scheduler`.
6. Implement `/api/v1/control/` and `/api/v1/org/` handlers.
7. Update browser handlers for org ownership checks.
8. Deploy: existing single-org deployments are unaffected (all accounts have `org_id=1`; all users have `org_id` matching their org; no cross-org calls exist). Org enforcement adds one DB check per browser start.
9. Rollback: remove `OrgScope` middleware and browser handler org checks; schema changes are backward-compatible.

## Open Questions

- Should API keys have an expiry date (`expires_at`)? Proposed: optional, `NULL` means never expires. Superadmin can force-revoke any key.
- Should `/api/v1/org/quota` return remaining capacity in real-time (DB + semaphore query) or cached (metrics snapshot)? Proposed: real-time for accuracy; cache can be added later.
- Should plan tier changes take effect immediately or at next billing cycle? Proposed: immediately (adjust semaphore capacity on next job submission); no billing logic in scope.

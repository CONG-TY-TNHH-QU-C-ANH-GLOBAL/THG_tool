## 1. Config and Database

- [ ] 1.1 Add `SUPERADMIN_EMAIL` and `DEFAULT_PLAN_TIER` (default `starter`) to `internal/config/config.go`
- [ ] 1.2 Add `.env.example` entries for both new vars with documentation
- [ ] 1.3 Add `api_keys` table migration in `internal/store/store.go`: `id`, `org_id`, `name`, `key_hash TEXT UNIQUE`, `role TEXT DEFAULT 'member'`, `created_at`, `last_used_at`, `revoked_at`, `expires_at`
- [ ] 1.4 Add `max_concurrent_browsers INT NOT NULL DEFAULT 0` and `warm_pool_size INT NOT NULL DEFAULT 0` columns to `organizations` via `ALTER TABLE … ADD COLUMN IF NOT EXISTS` (idempotent)
- [ ] 1.5 Add `superadmin` to the valid `role` values; update any `CHECK` constraint or Go-level validation that lists roles

## 2. Plan Tier Quotas

- [ ] 2.1 Create `internal/controlplane/quota.go` — define `PlanQuotas` struct and `PlanDefaults map[string]PlanQuotas` with `starter`, `growth`, `enterprise` entries
- [ ] 2.2 Add `EffectiveQuota() PlanQuotas` method to `Organization` model: returns per-org override if non-zero, else tier default
- [ ] 2.3 Write unit tests: `EffectiveQuota` returns override when set, tier default when zero, unknown tier returns starter defaults

## 3. Superadmin Bootstrap

- [ ] 3.1 Implement `store.SeedSuperadmin(email string) error` — `UPDATE users SET role='superadmin', org_id=0 WHERE email=?`; if no row updated, insert stub user with `crypto/rand` password hash
- [ ] 3.2 Call `store.SeedSuperadmin(cfg.SuperadminEmail)` in `cmd/scraper/main.go` startup sequence (after DB migrations, only if `SUPERADMIN_EMAIL` is set)
- [ ] 3.3 Write unit test: existing user gets promoted; missing user gets stub created

## 4. API Key Store

- [ ] 4.1 Create `internal/store/api_key_store.go` — `CreateAPIKey(orgID int64, name, role string) (plaintextKey string, err error)`: generate `thg_` + 32-byte base64url random, store `hex(SHA256(plaintext))`, return plaintext
- [ ] 4.2 Implement `ValidateAPIKey(bearerToken string) (*APIKey, error)` — hash the token, `SELECT * FROM api_keys WHERE key_hash=? AND revoked_at IS NULL`; update `last_used_at` asynchronously
- [ ] 4.3 Implement `ListAPIKeys(orgID int64) ([]APIKey, error)` — return all keys for org without `key_hash`
- [ ] 4.4 Implement `RevokeAPIKey(keyID, callerOrgID int64) error` — set `revoked_at=NOW()` only if `org_id=callerOrgID` (or caller is superadmin)
- [ ] 4.5 Write unit tests: create returns `thg_` prefix key, validate succeeds with correct key, validate fails after revoke, revoke from wrong org returns error, last_used_at updated

## 5. OrgScope Middleware

- [ ] 5.1 Create `internal/server/middleware_org_scope.go` — `OrgScope(store *store.Store, cfg config.Config) fiber.Handler`
- [ ] 5.2 In `OrgScope`: check `Authorization: Bearer` header; if value starts with `thg_` call `store.ValidateAPIKey`; else try JWT (`auth.ValidateAccessToken`); else try cookie `access_token`
- [ ] 5.3 On success inject `c.Locals("orgID", claims.OrgID)`, `c.Locals("role", claims.Role)`, `c.Locals("userID", claims.UserID)`; on failure return HTTP 401
- [ ] 5.4 For `role=superadmin` injecting `orgID=0` into Locals: add helper `OrgIDFromCtx(c) int64` that returns 0 for superadmin (callers treat 0 as "all orgs"); browser handlers must treat 0 differently (bypass org check, not restrict to nothing)
- [ ] 5.5 Wire `OrgScope` middleware to all existing auth-protected route groups in `internal/server/api.go`
- [ ] 5.6 Write unit tests: JWT auth path, API key auth path, revoked key rejected, missing auth rejected, superadmin gets orgID=0

## 6. OrgSemaphoreRegistry

- [ ] 6.1 Create `internal/browser/org_semaphore.go` — `OrgSemaphoreRegistry` struct with `mu sync.RWMutex` and `semaphores map[int64]chan struct{}`
- [ ] 6.2 Implement `Acquire(orgID int64, maxConcurrent int) error` — get or lazily create `chan struct{}` of capacity `maxConcurrent` for that org; non-blocking try-send; return error if full
- [ ] 6.3 Implement `Release(orgID int64)` — receive from the org's channel (release slot)
- [ ] 6.4 Implement `ActiveCount(orgID int64) int` — return `cap(ch) - len(ch)` for the org's channel
- [ ] 6.5 Replace the global scheduler semaphore `chan struct{}` in `internal/browser/scheduler.go` with an `*OrgSemaphoreRegistry`; pass `orgID` and `org.EffectiveQuota().MaxConcurrentBrowsers` to `Acquire` when a worker processes a job
- [ ] 6.6 Update `Job` struct to carry `OrgID int64`; set it in `JobQueue.Submit(accountID, orgID int64)`
- [ ] 6.7 Write unit tests: two orgs have independent caps, Org A at cap does not block Org B, Release frees slot

## 7. Browser Handler Org Enforcement

- [ ] 7.1 In `POST /browser/start` handler: call `store.GetAccount(accountID)` to fetch `account.org_id`; compare to `OrgIDFromCtx(c)`; return HTTP 403 if mismatch (skip check if caller is superadmin)
- [ ] 7.2 Pass `orgID` from context to `Scheduler.Submit(accountID, orgID)`
- [ ] 7.3 In `POST /browser/stop` handler: same org-ownership check before stopping
- [ ] 7.4 In `GET /browser/:id/status` handler: same org-ownership check before returning status
- [ ] 7.5 Add `store.GetAccountOrgID(accountID int64) (int64, error)` helper to avoid loading the full account record on every check

## 8. Control Plane Handlers (Superadmin)

- [ ] 8.1 Create `internal/server/control_handlers.go` — `ControlHandlers` struct with store reference
- [ ] 8.2 Implement `GET /api/v1/control/orgs` — `store.GetAllOrgs()` with effective quota + current browser/account counts
- [ ] 8.3 Implement `POST /api/v1/control/orgs` — create org with given `name`, `domain`, `plan_tier`
- [ ] 8.4 Implement `PATCH /api/v1/control/orgs/:id` — update `plan_tier`, `max_concurrent_browsers`, `warm_pool_size`; on quota change update `OrgSemaphoreRegistry` capacity
- [ ] 8.5 Implement `GET /api/v1/control/users` — list all users with org info
- [ ] 8.6 Implement `PATCH /api/v1/control/users/:id` — reassign org and role
- [ ] 8.7 Add `superadmin` role guard middleware: `RequireSuperadmin() fiber.Handler` — checks `c.Locals("role") == "superadmin"`, returns HTTP 403 otherwise
- [ ] 8.8 Register all control plane routes in `api.go` under `/api/v1/control/` with `RequireSuperadmin` middleware

## 9. Org Admin Handlers

- [ ] 9.1 Create `internal/server/org_handlers.go` — `OrgHandlers` struct with store reference
- [ ] 9.2 Implement `GET /api/v1/org/members` — `store.GetUsersByOrg(orgID)`
- [ ] 9.3 Implement `POST /api/v1/org/members` — create user in caller org with given email + role; return temp password
- [ ] 9.4 Implement `DELETE /api/v1/org/members/:user_id` — verify user belongs to caller org; soft-delete or unassign
- [ ] 9.5 Implement `GET /api/v1/org/api-keys` — `store.ListAPIKeys(orgID)`
- [ ] 9.6 Implement `POST /api/v1/org/api-keys` — `store.CreateAPIKey(orgID, name, role)` (admin+ only)
- [ ] 9.7 Implement `DELETE /api/v1/org/api-keys/:key_id` — `store.RevokeAPIKey(keyID, callerOrgID)` (admin+ only)
- [ ] 9.8 Implement `GET /api/v1/org/quota` — fetch org, call `EffectiveQuota()`, join with live semaphore count and DB account count
- [ ] 9.9 Implement `GET /api/v1/org/profile` — return org name, domain, plan_tier for caller's org
- [ ] 9.10 Add `RequireAdmin() fiber.Handler` — checks `role` is `admin` or `superadmin`; returns HTTP 403 otherwise
- [ ] 9.11 Register all org admin routes in `api.go` under `/api/v1/org/` with `OrgScope` + appropriate role guards

## 10. Existing API Org Scoping

- [ ] 10.1 Verify `GET /api/accounts` uses `orgID` from context in `store.GetAllAccounts(orgID)` (already partially implemented — confirm and patch gaps)
- [ ] 10.2 Verify `GET /api/groups` uses `orgID` in `store.GetAllGroups(orgID)` — confirm and patch
- [ ] 10.3 Add org check to `POST /api/accounts` — reject if `current_accounts >= org.EffectiveQuota().MaxAccounts`
- [ ] 10.4 Add `store.CountAccountsByOrg(orgID int64) (int, error)` helper used by quota check

## 11. Verification

- [ ] 11.1 `go build ./cmd/scraper/` passes with no new warnings
- [ ] 11.2 `go test ./internal/controlplane/... ./internal/store/... ./internal/browser/...` — all new tests pass with `-race`
- [ ] 11.3 Manual test: create two orgs via control plane, assign accounts to each, log in as Org A user, attempt `POST /browser/start` with Org B `account_id` — confirm HTTP 403
- [ ] 11.4 Manual test: issue API key via `POST /api/v1/org/api-keys`, use it in `Authorization: Bearer thg_…` header for `POST /browser/start` — confirm it works; revoke key, confirm HTTP 401
- [ ] 11.5 Manual test: set Org A `max_concurrent_browsers=2` via control plane, start 2 containers for Org A, verify third goes to queue; start container for Org B, verify Org B proceeds immediately
- [ ] 11.6 Manual test: call `GET /api/v1/org/quota` and confirm `current_browsers` matches running container count

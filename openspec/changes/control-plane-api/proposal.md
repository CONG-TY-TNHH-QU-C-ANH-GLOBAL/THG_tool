## Why

The system already seeds an `organizations` table and stamps `org_id` on users, accounts, and groups — but the browser APIs (`/browser/start`, `/browser/stop`, scheduler, warm pool) are completely org-blind. Any authenticated staff user can start a container for any account regardless of org membership. As the platform grows to serve multiple client organizations, every browser operation must be scoped to the caller's `org_id`, programmatic callers need API keys (not just browser-session JWTs), and superadmin needs a control plane to provision orgs and enforce per-org quotas.

## What Changes

- Introduce **API keys** as a first-class authentication method alongside JWT: each org can issue named API keys that carry `org_id` + `role` claims; keys are stored as SHA-256 hashes; the plaintext is shown once on creation.
- Introduce a **Control Plane API** under `/api/v1/control/` (superadmin-only) for org CRUD, user management across orgs, and quota enforcement.
- **Scope all browser APIs** to the caller's `org_id`: `POST /browser/start` may only target accounts that belong to the caller's org; the scheduler and warm pool enforce per-org `max_concurrent_browsers` quotas instead of a single global cap.
- Add an **Org Admin API** under `/api/v1/org/` (org `admin` role) for managing the caller's own org: invite users, list accounts, rotate API keys, view quota usage.
- **BREAKING**: `POST /browser/start` request body gains a required `account_id` field validated against the caller's org. Requests for out-of-org accounts return HTTP 403.
- Formalize `organizations.plan_tier` into three tiers (`starter`, `growth`, `enterprise`) with documented per-tier defaults for `max_concurrent_browsers`, `max_accounts`, and `warm_pool_size`.

## Capabilities

### New Capabilities

- `api-key-auth`: Issue, validate, revoke, and list API keys per org; keys authenticate browser and org-admin API calls as an alternative to JWT cookies.
- `tenant-quotas`: Per-org enforcement of `max_concurrent_browsers`, `max_accounts`, and `warm_pool_size` drawn from the org's `plan_tier`; quota headroom exposed via `/api/v1/org/quota`.
- `control-plane-org-management`: Superadmin CRUD for organizations, plan assignment, user assignment across orgs, and global quota override.
- `org-admin-api`: Self-service API for org admins: list/invite/remove members, view quota usage, manage API keys, list their accounts.

### Modified Capabilities

- `browser-container-lifecycle`: `POST /browser/start` and `POST /browser/stop` now enforce org ownership of the target `account_id`; requests for foreign accounts return HTTP 403. Per-org `max_concurrent_browsers` replaces the global cap. Requires delta spec.
- `browser-job-queue`: Concurrency cap lookup changes from a static config value to `org.max_concurrent_browsers` fetched from the DB at job submission time. Requires delta spec.

## Impact

- **Code**: New `internal/controlplane/` package (`org_store.go`, `api_key_store.go`, `quota.go`); new `internal/server/control_handlers.go` and `org_handlers.go`; `internal/auth/auth.go` extended to validate API keys; all browser handlers updated to extract `orgID` from context and validate account ownership.
- **APIs**: New routes under `/api/v1/control/` and `/api/v1/org/`; `POST /browser/start` body change (BREAKING); existing `/api/` routes gain org-scope enforcement (non-breaking for single-org deployments).
- **Database**: `organizations` table gains `max_concurrent_browsers INT`, `warm_pool_size INT` columns; new `api_keys` table (`id`, `org_id`, `name`, `key_hash`, `role`, `created_at`, `last_used_at`, `revoked_at`).
- **Config**: `SUPERADMIN_EMAIL` env var to designate bootstrap superadmin; `DEFAULT_PLAN_TIER` (default `starter`) for new orgs.
- **No new external dependencies** — uses existing SQLite, JWT, and bcrypt infrastructure.

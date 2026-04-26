## ADDED Requirements

### Requirement: Plan tier default quotas
The system SHALL define three plan tiers with fixed default quotas. These defaults apply to any org whose `organizations` column override is zero (unset).

| Tier | `max_concurrent_browsers` | `max_accounts` | `warm_pool_size` |
|---|---|---|---|
| `starter` | 3 | 5 | 1 |
| `growth` | 20 | 30 | 5 |
| `enterprise` | 100 | 200 | 20 |

#### Scenario: Starter org uses tier defaults
- **WHEN** an org has `plan_tier='starter'` and `max_concurrent_browsers=0` (unset override)
- **THEN** `org.EffectiveQuota().MaxConcurrentBrowsers` returns 3

#### Scenario: Override takes precedence over tier default
- **WHEN** an org has `plan_tier='starter'` and `max_concurrent_browsers=10`
- **THEN** `org.EffectiveQuota().MaxConcurrentBrowsers` returns 10

### Requirement: Per-org concurrent browser cap enforcement
The system SHALL enforce `org.EffectiveQuota().MaxConcurrentBrowsers` as the concurrency limit for each org independently. An org at its cap SHALL NOT be blocked by another org's usage, and SHALL NOT be able to consume another org's capacity.

#### Scenario: Org A at cap does not affect Org B
- **WHEN** Org A has 3 running containers (at its starter cap of 3) and Org B submits a browser start request
- **THEN** Org B's request proceeds normally using Org B's own semaphore slot

#### Scenario: Org at cap returns queued job
- **WHEN** an org's `OrgSemaphoreRegistry` slot count equals `MaxConcurrentBrowsers` and a new `POST /browser/start` is called for that org
- **THEN** the job is enqueued in the scheduler queue; the semaphore blocks the worker until a slot is freed (behavior unchanged from `browser-scheduler` change)

### Requirement: Quota usage visibility endpoint
The system SHALL expose `GET /api/v1/org/quota` returning the caller org's effective quota limits and current usage.

#### Scenario: Quota returned for authenticated org
- **WHEN** `GET /api/v1/org/quota` is called by an org member
- **THEN** the response is HTTP 200 with `{ "org_id", "plan_tier", "max_concurrent_browsers", "current_browsers", "max_accounts", "current_accounts", "warm_pool_size" }`

#### Scenario: Superadmin sees all orgs quota summary
- **WHEN** `GET /api/v1/control/orgs` is called by a superadmin
- **THEN** each org entry in the response includes its `current_browsers` and `current_accounts` alongside quota limits

### Requirement: Account count cap enforcement
The system SHALL reject requests to create a new Facebook account via `POST /api/accounts` when the org's current account count equals `org.EffectiveQuota().MaxAccounts`.

#### Scenario: Account creation rejected at cap
- **WHEN** an org at `max_accounts=5` calls `POST /api/accounts` and already has 5 accounts
- **THEN** the response is HTTP 429 with `{ "error": "account limit reached", "limit": 5 }`

#### Scenario: Account creation allowed below cap
- **WHEN** an org has 4 accounts and `max_accounts=5`
- **THEN** `POST /api/accounts` creates the account normally

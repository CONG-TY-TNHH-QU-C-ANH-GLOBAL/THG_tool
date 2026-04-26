## ADDED Requirements

### Requirement: Superadmin org CRUD
The system SHALL expose org management endpoints under `/api/v1/control/orgs/` accessible only to users with `role=superadmin`. These endpoints allow creating, reading, updating, and listing all organizations.

#### Scenario: Create new org
- **WHEN** a superadmin calls `POST /api/v1/control/orgs` with `{ "name": "Acme Corp", "domain": "acme.com", "plan_tier": "growth" }`
- **THEN** a new org is created with the given plan tier and default quotas; response is HTTP 201 with the full org record including `id`

#### Scenario: List all orgs
- **WHEN** a superadmin calls `GET /api/v1/control/orgs`
- **THEN** all organizations are returned with their `id`, `name`, `domain`, `plan_tier`, effective quota values, and current account/browser counts

#### Scenario: Update org plan tier
- **WHEN** a superadmin calls `PATCH /api/v1/control/orgs/:id` with `{ "plan_tier": "enterprise" }`
- **THEN** the org's `plan_tier` is updated; `EffectiveQuota()` reflects the new tier defaults on the next quota lookup

#### Scenario: Override org quota beyond tier default
- **WHEN** a superadmin calls `PATCH /api/v1/control/orgs/:id` with `{ "max_concurrent_browsers": 150 }`
- **THEN** `organizations.max_concurrent_browsers` is set to 150; `EffectiveQuota()` returns 150 regardless of plan tier

#### Scenario: Non-superadmin blocked from control plane
- **WHEN** a request with `role=admin` or `role=sales` is made to any `/api/v1/control/` route
- **THEN** the response is HTTP 403 with `{ "error": "superadmin required" }`

### Requirement: Superadmin user management across orgs
The system SHALL allow superadmins to assign users to orgs, change user roles, and view all users regardless of org.

#### Scenario: Assign user to org
- **WHEN** a superadmin calls `PATCH /api/v1/control/users/:id` with `{ "org_id": 3, "role": "admin" }`
- **THEN** the user's `org_id` and `role` are updated; their JWT on next login reflects the new values

#### Scenario: List all users with org info
- **WHEN** a superadmin calls `GET /api/v1/control/users`
- **THEN** all users are returned with `id`, `email`, `role`, `org_id`, `org_name`

### Requirement: Superadmin bootstrap
The system SHALL promote the user matching `SUPERADMIN_EMAIL` to `role=superadmin, org_id=0` on every startup. If no user with that email exists, a stub record is created with a random password hash.

#### Scenario: Existing user promoted on startup
- **WHEN** `SUPERADMIN_EMAIL=admin@thgfulfill.com` is set and a user with that email exists
- **THEN** on startup their `role` is set to `superadmin` and `org_id` to 0

#### Scenario: SUPERADMIN_EMAIL not set — no bootstrap
- **WHEN** `SUPERADMIN_EMAIL` is not configured
- **THEN** no superadmin bootstrap runs; the system starts normally with existing user roles

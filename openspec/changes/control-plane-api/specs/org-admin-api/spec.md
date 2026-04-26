## ADDED Requirements

### Requirement: Org member management by admin
The system SHALL allow org admins to list, invite, and remove members within their own organization. Admins SHALL NOT see or modify users in other orgs.

#### Scenario: List org members
- **WHEN** `GET /api/v1/org/members` is called by an org admin
- **THEN** all users with `org_id` matching the caller's org are returned with `id`, `email`, `role`, `created_at`

#### Scenario: Invite new member
- **WHEN** `POST /api/v1/org/members` is called with `{ "email": "newuser@example.com", "role": "sales" }`
- **THEN** a new user is created in the caller's org with that email and role; a temporary password is generated and returned in the response for the admin to share; quota cap on users is not enforced (user count is not capped)

#### Scenario: Remove member from org
- **WHEN** `DELETE /api/v1/org/members/:user_id` is called by an org admin
- **THEN** the user is soft-deleted (or `org_id` set to 0); they can no longer log in

#### Scenario: Admin cannot modify users in other orgs
- **WHEN** `DELETE /api/v1/org/members/:user_id` is called for a user in a different org
- **THEN** the response is HTTP 403

### Requirement: Org API key management
The system SHALL allow org admins to create, list, and revoke API keys for their organization via `/api/v1/org/api-keys/` endpoints (delegated from the `api-key-auth` capability).

#### Scenario: Admin lists org API keys
- **WHEN** `GET /api/v1/org/api-keys` is called
- **THEN** all API keys for the caller's org are returned, without plaintext values

#### Scenario: Sales role cannot manage API keys
- **WHEN** a user with `role=sales` calls `POST /api/v1/org/api-keys`
- **THEN** the response is HTTP 403 with `{ "error": "admin role required" }`

### Requirement: Org quota visibility
The system SHALL expose the org's current quota usage at `GET /api/v1/org/quota` to any authenticated member of the org (any role).

#### Scenario: Member views quota
- **WHEN** `GET /api/v1/org/quota` is called by a sales-role user
- **THEN** the response includes effective limits and current counts (no admin role required for read)

### Requirement: Org profile read
The system SHALL expose `GET /api/v1/org/profile` returning the caller's org name, domain, and plan tier. Only org members may read their own org profile; they cannot read other orgs.

#### Scenario: Member reads own org profile
- **WHEN** `GET /api/v1/org/profile` is called by any authenticated org member
- **THEN** the response is HTTP 200 with `{ "id", "name", "domain", "plan_tier" }`

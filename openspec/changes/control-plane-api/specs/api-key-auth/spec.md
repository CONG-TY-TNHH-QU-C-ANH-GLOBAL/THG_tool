## ADDED Requirements

### Requirement: API key issuance
The system SHALL allow org admins and superadmins to issue named API keys for their organization. Each key SHALL be generated as a cryptographically random 32-byte value encoded as `thg_<base64url>`, stored as a SHA-256 hash, and returned in plaintext exactly once at creation time.

#### Scenario: API key created successfully
- **WHEN** `POST /api/v1/org/api-keys` is called by an org admin with `{ "name": "CI deploy key" }`
- **THEN** the response is HTTP 201 with `{ "key_id", "name", "plaintext_key": "thg_...", "org_id", "role": "member", "created_at" }`; subsequent calls do NOT return the plaintext again

#### Scenario: Plaintext not retrievable after creation
- **WHEN** `GET /api/v1/org/api-keys` is called
- **THEN** the response lists keys with `key_id`, `name`, `created_at`, `last_used_at`, `revoked_at` — no `plaintext_key` field

#### Scenario: Key name must be unique within org
- **WHEN** `POST /api/v1/org/api-keys` is called with a name already used by an active key in the same org
- **THEN** the response is HTTP 409 with `{ "error": "key name already exists" }`

### Requirement: API key authentication on protected endpoints
The system SHALL accept `Authorization: Bearer thg_<key>` as a valid authentication method on all endpoints that accept JWT auth. The system SHALL resolve the key to its `org_id` and `role`, injecting them into the request context identically to a JWT claim.

#### Scenario: Valid API key authenticates request
- **WHEN** a request to `POST /browser/start` includes `Authorization: Bearer thg_<valid_key>`
- **THEN** the request is authenticated as the key's org and role; `org_id` is extracted and used for all downstream org-scoping checks

#### Scenario: Revoked API key rejected
- **WHEN** a request includes `Authorization: Bearer thg_<revoked_key>` (where `revoked_at IS NOT NULL`)
- **THEN** the response is HTTP 401 with `{ "error": "API key revoked" }`

#### Scenario: Unknown API key rejected
- **WHEN** a request includes `Authorization: Bearer thg_<hash_not_in_db>`
- **THEN** the response is HTTP 401 with `{ "error": "invalid API key" }`

#### Scenario: last_used_at updated on use
- **WHEN** a valid API key is used to authenticate a request
- **THEN** `api_keys.last_used_at` is updated to the current UTC timestamp (best-effort, non-blocking)

### Requirement: API key revocation
The system SHALL allow org admins and superadmins to revoke any API key belonging to their org. Revoked keys SHALL be immediately rejected; they are not deleted from the DB to preserve audit trail.

#### Scenario: Key revoked by org admin
- **WHEN** `DELETE /api/v1/org/api-keys/:key_id` is called by an org admin for a key in their org
- **THEN** `api_keys.revoked_at` is set to the current timestamp; subsequent requests using that key return HTTP 401

#### Scenario: Admin cannot revoke key from another org
- **WHEN** `DELETE /api/v1/org/api-keys/:key_id` is called by an org admin for a key in a different org
- **THEN** the response is HTTP 403

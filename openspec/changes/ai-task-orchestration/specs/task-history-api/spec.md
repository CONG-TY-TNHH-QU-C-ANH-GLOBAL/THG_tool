## ADDED Requirements

### Requirement: Single task status endpoint
The system SHALL expose `GET /api/v1/tasks/:id` that returns the full `tasks` row for the given task ID. The response SHALL include `id`, `account_id`, `org_id`, `skill`, `params_json`, `status`, `summary`, `result_json`, `error`, `prompt_tokens`, `completion_tokens`, `duration_ms`, `created_at`, `updated_at`.

#### Scenario: Task found
- **WHEN** `GET /api/v1/tasks/42` is called by a user whose org matches the task's `org_id`
- **THEN** the system returns HTTP 200 with the full task record as JSON

#### Scenario: Task not found
- **WHEN** `GET /api/v1/tasks/9999` is called and no task with that ID exists
- **THEN** the system returns HTTP 404 with `{"error":"task not found"}`

#### Scenario: Task belonging to another org rejected
- **WHEN** `GET /api/v1/tasks/42` is called by a user whose `org_id` does not match the task's `org_id` (and caller is not superadmin)
- **THEN** the system returns HTTP 403 with `{"error":"task not found"}` (intentionally opaque)

### Requirement: Task list endpoint
The system SHALL expose `GET /api/v1/tasks` that returns a paginated list of tasks for the caller's org. Optional query params: `account_id`, `skill`, `status`, `limit` (default 50, max 200), `offset` (default 0). Response format: `{"tasks":[...],"total":N,"limit":N,"offset":N}`.

#### Scenario: List tasks for org
- **WHEN** `GET /api/v1/tasks` is called with no filters
- **THEN** the system returns tasks where `org_id` matches the caller's org, ordered by `created_at DESC`, up to `limit`

#### Scenario: Filter by skill and status
- **WHEN** `GET /api/v1/tasks?skill=scrape_group&status=completed` is called
- **THEN** only tasks with `skill='scrape_group' AND status='completed'` for the caller's org are returned

#### Scenario: Superadmin sees all orgs
- **WHEN** `GET /api/v1/tasks` is called by a superadmin (orgID=0)
- **THEN** tasks from all orgs are returned without org filtering

### Requirement: Task fields in list response
The `GET /api/v1/tasks` list response SHALL return the same fields as `GET /api/v1/tasks/:id` for each task object except `result_json` (omitted from the list to keep response size bounded). `result_json` is available only via the single-item endpoint.

#### Scenario: result_json omitted from list
- **WHEN** `GET /api/v1/tasks` returns a list of tasks
- **THEN** each task object in the `"tasks"` array does NOT contain `result_json`; all other fields are present

### Requirement: Auth and route registration
Both task history endpoints SHALL be protected by the existing `OrgScope` middleware. They SHALL be registered under `/api/v1/tasks/` in `internal/server/api.go`.

#### Scenario: Unauthenticated request rejected
- **WHEN** `GET /api/v1/tasks` is called without a valid `Authorization` header or session cookie
- **THEN** `OrgScope` returns HTTP 401 before the handler is reached

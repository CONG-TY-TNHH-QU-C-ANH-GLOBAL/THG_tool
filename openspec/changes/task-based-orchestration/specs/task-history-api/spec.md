## MODIFIED Requirements

### Requirement: Single task status endpoint
The system SHALL expose `GET /api/v1/tasks/:id` that returns the task record for the given `task_id` (hex string). The response SHALL include: `task_id`, `intent`, `org_id`, `account_id`, `status`, `result` (the `OutputDataset` with `total_fetched` omitted), `error`, `parse_prompt_tokens`, `parse_completion_tokens`, `insight_prompt_tokens`, `insight_completion_tokens`, `duration_ms`, `created_at`, `updated_at`. The response SHALL NOT include the raw `task_json` field (to avoid leaking crawl strategy details).

#### Scenario: Completed task with results
- **WHEN** `GET /api/v1/tasks/a3f7c2e1b9d40851` is called and the task is `completed`
- **THEN** HTTP 200 with `result.records`, `result.stats.total_returned`, `result.insights`, and token usage fields; `result.stats.total_fetched` is omitted; `task_json` is omitted

#### Scenario: Pending task returns status only
- **WHEN** `GET /api/v1/tasks/a3f7c2e1b9d40851` is called and `status="pending"`
- **THEN** HTTP 200 with `{task_id, intent, status:"pending", result:null}`

#### Scenario: Cross-org access returns 403 as 404
- **WHEN** the caller's `org_id` does not match the task's `org_id`
- **THEN** HTTP 403 returned as `{"error":"task not found"}` (opaque, no org info leaked)

### Requirement: Task list endpoint with intent and status filters
The system SHALL expose `GET /api/v1/tasks` that accepts: `intent` (filter by task type), `status` (filter by status), `account_id` (filter by account), `limit` (default 50, max 200), `offset` (default 0). Response: `{"tasks":[...],"total":N,"limit":N,"offset":N}`. The list items omit `result` entirely (reference the `:id` endpoint for results).

#### Scenario: List all completed lead_generation tasks for org
- **WHEN** `GET /api/v1/tasks?intent=lead_generation&status=completed` is called
- **THEN** tasks matching both filters for the caller's org are returned, ordered by `created_at DESC`

#### Scenario: result field absent from list
- **WHEN** `GET /api/v1/tasks` returns a list
- **THEN** each task object contains `task_id`, `intent`, `status`, `duration_ms`, `created_at`, `updated_at` only; `result` and `task_json` are absent

## REMOVED Requirements

### Requirement: Skill-domain task fields in API response
**Reason**: `skill`, `params_json`, `summary` columns no longer exist in the `tasks` table. These were part of the skill-based execution model which is removed.
**Migration**: Clients reading `skill` or `params_json` from task API responses should use `intent` and `task_json` (internal only) respectively. Summary is now `result.insights.summary` for tasks that request the summary insight.

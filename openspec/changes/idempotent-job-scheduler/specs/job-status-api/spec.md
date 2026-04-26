## ADDED Requirements

### Requirement: Single job status endpoint
The system SHALL expose `GET /api/v1/jobs/:id` that returns the full job record for the given job ID. The response SHALL include `id`, `type`, `idempotency_key`, `status`, `attempt`, `max_attempts`, `run_after`, `claimed_by`, `created_at`, `updated_at`, and `error`.

#### Scenario: Job found
- **WHEN** `GET /api/v1/jobs/42` is called and a job with `id=42` exists
- **THEN** the system returns HTTP 200 with the job's full record as JSON

#### Scenario: Job not found
- **WHEN** `GET /api/v1/jobs/9999` is called and no job with that ID exists
- **THEN** the system returns HTTP 404 with `{ "error": "job not found" }`

### Requirement: Job list endpoint
The system SHALL expose `GET /api/v1/jobs` that returns a paginated list of jobs filtered by optional query parameters `type`, `status`, `limit` (default 50, max 200), and `offset` (default 0). The response SHALL be `{ "jobs": [...], "total": N, "limit": N, "offset": N }`.

#### Scenario: List all jobs with default pagination
- **WHEN** `GET /api/v1/jobs` is called with no query parameters
- **THEN** the system returns up to 50 jobs ordered by `created_at DESC` with `total` reflecting the count of all jobs

#### Scenario: Filter by type and status
- **WHEN** `GET /api/v1/jobs?type=browser_start&status=pending` is called
- **THEN** the system returns only jobs with `type='browser_start'` AND `status='pending'`

#### Scenario: Pagination with offset
- **WHEN** `GET /api/v1/jobs?limit=10&offset=20` is called
- **THEN** the system skips the first 20 jobs and returns up to 10 jobs; `offset` and `limit` are reflected in the response envelope

#### Scenario: Limit capped at maximum
- **WHEN** `GET /api/v1/jobs?limit=500` is called
- **THEN** the system applies `limit=200` (the cap) and returns at most 200 jobs

### Requirement: Job list fields
The `GET /api/v1/jobs` list response SHALL return the same fields as `GET /api/v1/jobs/:id` for each job object; no field omission in the list view.

#### Scenario: Full fields in list response
- **WHEN** `GET /api/v1/jobs` returns a non-empty job list
- **THEN** each job object in the `"jobs"` array contains `id`, `type`, `idempotency_key`, `status`, `attempt`, `max_attempts`, `run_after`, `claimed_by`, `created_at`, `updated_at`, and `error`

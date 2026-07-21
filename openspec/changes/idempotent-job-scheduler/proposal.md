> **Lifecycle status (2026-07-21 spec IA reconciliation):** proposal only — nothing under `openspec/` is current runtime authority (per `AGENTS.md`/`CLAUDE.md`; the runtime authority is `specs/domains/platform-foundation/features/runtime-topology/technical.md`). Durable idempotent job-queue semantics HAVE shipped in a different shape: `internal/jobs/store.go` (scheduler_jobs table, `INSERT OR IGNORE` on task_id, atomic claim UPDATE, `retry_after` stale recovery, progress/finish states). The task list below as written (idempotency_key/claimed_by column shape) was NOT executed — PARTIALLY REALIZED, different shape; remaining deltas need re-proposal against the current runtime.

## Why

The `browser-scheduler` change (prior) uses an in-memory `chan *Job` queue that loses all pending work on service restart and has no protection against a job being picked up by two workers simultaneously if the channel is ever shared across goroutines incorrectly. A general-purpose SQLite-backed job scheduler with a strong idempotency key guarantee and single-source execution (exactly one worker claims each job, enforced at the DB level) gives the browser platform — and future subsystems — a durable, restart-safe job queue without an external dependency.

## What Changes

- Introduce a `Scheduler` component backed by a SQLite `jobs` table (reusing the existing DB) with columns for `type`, `idempotency_key`, `payload` (JSON), `status`, `attempt`, `max_attempts`, `run_after`, `claimed_by`, `created_at`, `updated_at`.
- Job submission is idempotent per `(type, idempotency_key)`: submitting a job whose key already exists in a non-terminal state returns the existing job with no side effects.
- Workers claim jobs via `UPDATE jobs SET status='running', claimed_by=? WHERE id=? AND status='pending' RETURNING *` — SQLite's exclusive write lock guarantees exactly one worker wins per job row.
- Pluggable handler registry: each job type maps to a `JobHandler` Go function; unknown types are rejected at submission time.
- Configurable retry policy per job type: `max_attempts`, `backoff_strategy` (`constant` or `exponential`), `retry_delay`.
- Stale job recovery: a background goroutine detects jobs stuck in `running` state beyond `claimed_timeout` and resets them to `pending`.
- REST visibility: `GET /api/v1/jobs/:id` returns job status; `GET /api/v1/jobs?type=X&status=Y` lists jobs.

## Capabilities

### New Capabilities

- `job-store`: SQLite-backed persistent job table with `(type, idempotency_key)` uniqueness, atomic claim via `UPDATE … RETURNING`, and stale job recovery.
- `job-handler-registry`: Type-safe registry mapping job type strings to `JobHandler` functions with per-type retry policy.
- `job-scheduler-worker`: Worker pool that polls the job store, claims and executes jobs, handles retries, and marks terminal states.
- `job-status-api`: REST endpoints to query job status and list jobs by type/status.

### Modified Capabilities

- `browser-job-queue`: The in-memory `chan *Job` and `sync.Map` job store are replaced by the new `job-store` and `job-scheduler-worker` backends. Idempotency key becomes `browser_start:account:<id>`. Response shape is unchanged. Requires delta spec.

## Impact

- **Code**: New `internal/jobs/` package (`store.go`, `registry.go`, `worker.go`, `api.go`); `internal/browser/scheduler.go` and `job_queue.go` replaced by thin wrappers calling `internal/jobs`; new `GET /api/v1/jobs/` routes in `api.go`.
- **Database**: New `jobs` table in the existing SQLite DB (no new file or driver).
- **APIs**: Two new read-only REST endpoints; no breaking changes to `POST /browser/start` or `GET /browser/jobs/:id` — same response shapes, now backed by persisted rows.
- **Dependencies**: No new external dependencies — uses existing `modernc.org/sqlite` and standard library.
- **Config**: `JOB_WORKER_COUNT` (default 4), `JOB_POLL_INTERVAL` (default `500ms`), `JOB_CLAIMED_TIMEOUT` (default `5m`), `JOB_MAX_RETENTION` (default `24h` — terminal jobs older than this are purged).

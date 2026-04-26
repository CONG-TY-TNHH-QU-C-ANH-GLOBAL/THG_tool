## Context

The existing `browser-scheduler` change implements an in-memory job queue (`chan *Job`) with a `sync.Map` job store. It handles the browser start flow well at small scale but has two fundamental weaknesses:

1. **No durability**: Restarting the service drops all pending jobs. The browser accounts whose start was queued must be manually re-submitted.
2. **In-process exclusivity only**: The `UPDATE` logic that prevents double-execution is a mutex inside one Go process. If the binary is ever run as two instances (load balancer, blue-green deploy, crash-restart overlap), both instances can claim the same job.

SQLite's exclusive write semantics (`BEGIN IMMEDIATE` or row-level `UPDATE … WHERE status='pending' RETURNING *`) guarantee exactly-one-claimer even under concurrent readers — without an external message broker.

Existing infrastructure: SQLite via `modernc.org/sqlite`; a `jobs` table already exists in the store but is used for a different purpose (Chrome Extension agent jobs). The new `scheduler_jobs` table is separate to avoid schema collision.

## Goals / Non-Goals

**Goals:**
- `scheduler_jobs` SQLite table is the single source of truth for job state.
- `(type, idempotency_key)` uniqueness constraint: submitting the same logical job twice returns the existing row.
- Workers claim jobs via `UPDATE scheduler_jobs SET status='running', claimed_by=?, claimed_at=? WHERE id=(SELECT id FROM scheduler_jobs WHERE status='pending' AND run_after <= ? ORDER BY created_at LIMIT 1) RETURNING *` — SQLite's serialized writes enforce single-source execution.
- `JobHandler` interface: `Handle(ctx, job Job) error`. Registry maps type strings to handlers.
- Per-type retry policy: `MaxAttempts int`, `BackoffStrategy string`, `RetryDelay time.Duration`.
- Stale claim recovery: background goroutine resets jobs where `status='running' AND claimed_at < NOW - JOB_CLAIMED_TIMEOUT`.
- Terminal job retention and purge: jobs in `failed`/`completed` kept for `JOB_MAX_RETENTION`, then deleted.
- REST: `GET /api/v1/jobs/:id` and `GET /api/v1/jobs?type=&status=&limit=`.

**Non-Goals:**
- Distributed lock via Redis or etcd — SQLite single-source is sufficient for single-node.
- Job priorities beyond `run_after` scheduling.
- Fan-out (one job → many workers).
- Cron/recurring jobs (separate concern).
- Replacing the existing `jobs` table used by the Chrome Extension agent.

## Decisions

### 1. Claim via subquery UPDATE — no advisory locks needed

**Decision**:
```sql
UPDATE scheduler_jobs
SET status='running', claimed_by=?, claimed_at=?
WHERE id = (
  SELECT id FROM scheduler_jobs
  WHERE status='pending' AND run_after <= ?
  ORDER BY created_at
  LIMIT 1
)
RETURNING *
```
SQLite holds an exclusive write lock for the duration of this statement. If two goroutines execute it simultaneously, one gets a row and the other gets zero rows (returns no job to process).

**Why**: No `SELECT FOR UPDATE` in SQLite; the subquery `UPDATE … WHERE id=(SELECT …)` is the standard SQLite idiom for atomic claim. It avoids the separate `BEGIN IMMEDIATE` transaction that would require connection-pool-aware locking.

**Alternative considered**: `BEGIN IMMEDIATE` + `SELECT` + `UPDATE` in a transaction — correct but requires holding the write lock across three statements; more code, same result.

### 2. Idempotency enforced by UNIQUE constraint + INSERT OR IGNORE

**Decision**: `scheduler_jobs` has `UNIQUE(type, idempotency_key)`. `Submit()` does:
```sql
INSERT OR IGNORE INTO scheduler_jobs (type, idempotency_key, payload, status, ...)
VALUES (?, ?, ?, 'pending', ...)
```
Then unconditionally fetches the row by `(type, idempotency_key)` to return current state. If the job was already running/completed/failed, `IGNORE` skips the insert and the fetch returns the existing row.

**Why**: DB-level uniqueness is atomic and survives concurrent callers. No application-level lock or `sync.Map` check needed. The fetch-after-insert pattern always returns the authoritative state.

**Alternative considered**: Check-then-insert with a mutex — TOCTOU between check and insert; rejected.

### 3. Worker pool polls on a ticker — no `LISTEN/NOTIFY`

**Decision**: Each worker goroutine loops: `tick → claim attempt → if nil job: sleep JOB_POLL_INTERVAL; else handle`. Workers do not coordinate — each independently polls the DB. `JOB_POLL_INTERVAL` defaults to 500ms.

**Why**: SQLite has no `LISTEN/NOTIFY` mechanism (that's PostgreSQL). A 500ms poll interval is imperceptible for background automation jobs. The claim query is fast (index on `status, run_after, created_at`).

**Alternative considered**: A shared Go channel that receives a signal on job submission — works within one process but breaks for future multi-process. Polling is simpler and process-agnostic.

### 4. `claimed_by` is a random instance UUID generated at startup

**Decision**: On startup, the scheduler generates a UUID (`instance_id`). All claims set `claimed_by = instance_id`. Stale claim recovery queries `claimed_at < NOW - CLAIMED_TIMEOUT` regardless of `claimed_by` — it resets the job to `pending`, not just the current instance's jobs.

**Why**: After a crash, `claimed_by` can't be queried because the process is gone. The timeout-based recovery reclaims stale jobs from any dead instance. The UUID allows distinguishing live claim vs. stale claim in observability.

### 5. `browser-scheduler` becomes a thin facade over the generic scheduler

**Decision**: `internal/browser/scheduler.go` is rewritten to: register a `"browser_start"` handler in `internal/jobs.Registry`; call `jobs.Submit("browser_start", fmt.Sprintf("account:%d", accountID), payload)` from the HTTP handler. The `JobQueue` and `Scheduler` structs in `internal/browser/` are deleted; `internal/jobs/` handles execution.

**Why**: No duplication. The browser start flow gets durability for free. Future job types (scrape, comment, inbox) use the same infrastructure.

## Risks / Trade-offs

- **SQLite write lock contention under high submit rate** → Mitigation: `JOB_WORKER_COUNT` defaults to 4; the claim query is fast (~1ms). At 100 jobs/s the lock hold time is <100ms total. For higher throughput, increase `JOB_WORKER_COUNT`; SQLite WAL mode (already set) allows concurrent readers during write.
- **Poll interval adds up to 500ms latency to job pickup** → Mitigation: For browser start (interactive), the warm pool masks most latency; cold starts already take 2–4s. 500ms is acceptable. Configurable to 100ms if needed.
- **Stale job recovery resets jobs that are still running on a slow worker** → Mitigation: `JOB_CLAIMED_TIMEOUT` defaults to 5 minutes, far above the ~4s cold container start. Set conservatively.
- **`UNIQUE(type, idempotency_key)` prevents reusing keys for retries** → Mitigation: Once a job reaches `failed` or `completed`, the unique index does not block resubmission — `INSERT OR IGNORE` inserts a new row because the old row with that key is deleted on terminal-job purge. Alternatively, `run_after` can be used as a retry discriminator.

## Migration Plan

1. Add `scheduler_jobs` table migration in `store.go`.
2. Implement `internal/jobs/` package.
3. Register `browser_start` handler in `main.go`.
4. Replace `internal/browser/scheduler.go` and `job_queue.go` with thin facade.
5. Update browser HTTP handlers to call `jobs.Submit(...)` directly.
6. Deploy: in-flight in-memory jobs are lost (same as today on restart); existing `jobs` table is untouched.
7. Rollback: restore old `scheduler.go` and `job_queue.go`; drop `scheduler_jobs` table (data loss of pending jobs only).

## Open Questions

- Should `scheduler_jobs` use WAL mode explicitly, or rely on the existing DB-level WAL setting? Proposed: inherit from existing connection (WAL already enabled in `store.go`).
- Should `GET /api/v1/jobs` be paginated? Proposed: yes, `limit=50` default, `offset` param.
- Should completed jobs be purged immediately or after `JOB_MAX_RETENTION`? Proposed: after retention (24h default) — useful for audit trail.

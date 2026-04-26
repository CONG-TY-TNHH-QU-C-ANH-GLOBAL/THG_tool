## 1. Database Migration

- [ ] 1.1 Add `scheduler_jobs` table migration in `internal/store/store.go`: columns `id`, `type`, `idempotency_key`, `payload TEXT`, `status TEXT`, `attempt INT DEFAULT 0`, `max_attempts INT DEFAULT 3`, `run_after DATETIME`, `claimed_by TEXT`, `claimed_at DATETIME`, `created_at`, `updated_at`, `error TEXT`
- [ ] 1.2 Add `UNIQUE(type, idempotency_key)` constraint on `scheduler_jobs`
- [ ] 1.3 Add index `idx_scheduler_jobs_claim` on `(status, run_after, created_at)` to support the claim query efficiently
- [ ] 1.4 Verify migration is idempotent (uses `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`)

## 2. Job Store (`internal/jobs/store.go`)

- [ ] 2.1 Define `Job` struct with all `scheduler_jobs` fields as Go types (`Status`, `Type`, `IdempotencyKey`, `Payload`, `Attempt`, `MaxAttempts`, `RunAfter`, `ClaimedBy`, `ClaimedAt`, `CreatedAt`, `UpdatedAt`, `Error`)
- [ ] 2.2 Implement `Submit(jobType, idempotencyKey string, payload any) (*Job, error)`: `INSERT OR IGNORE … VALUES ('pending', …)` then `SELECT … WHERE type=? AND idempotency_key=?`; return error if type not registered
- [ ] 2.3 Implement `Claim(instanceID string, now time.Time) (*Job, error)`: single subquery `UPDATE scheduler_jobs SET status='running', claimed_by=?, claimed_at=? WHERE id=(SELECT id FROM scheduler_jobs WHERE status='pending' AND run_after <= ? ORDER BY created_at LIMIT 1) RETURNING *`
- [ ] 2.4 Implement `Complete(jobID int64) error`: `UPDATE scheduler_jobs SET status='completed', updated_at=? WHERE id=?`
- [ ] 2.5 Implement `Fail(jobID int64, errMsg string) error`: `UPDATE scheduler_jobs SET status='failed', error=?, updated_at=? WHERE id=?`
- [ ] 2.6 Implement `Retry(jobID int64, attempt int, runAfter time.Time, errMsg string) error`: `UPDATE scheduler_jobs SET status='pending', attempt=?, run_after=?, error=?, claimed_by=NULL, claimed_at=NULL, updated_at=? WHERE id=?`
- [ ] 2.7 Implement `RecoverStale(timeout time.Duration) (int64, error)`: `UPDATE scheduler_jobs SET status='pending', claimed_by=NULL, claimed_at=NULL WHERE status='running' AND claimed_at < ?`; return rows affected
- [ ] 2.8 Implement `PurgeTerminal(retention time.Duration) (int64, error)`: `DELETE FROM scheduler_jobs WHERE status IN ('failed','completed') AND updated_at < ?`; return rows affected
- [ ] 2.9 Implement `GetByID(id int64) (*Job, error)` and `List(jobType, status string, limit, offset int) ([]Job, int, error)` for the API

## 3. Handler Registry (`internal/jobs/registry.go`)

- [ ] 3.1 Define `JobHandler` interface: `Handle(ctx context.Context, job Job) error`
- [ ] 3.2 Define `RetryPolicy` struct: `MaxAttempts int`, `BackoffStrategy string` (`"constant"` or `"exponential"`), `RetryDelay time.Duration`
- [ ] 3.3 Define `entry` struct holding `handler JobHandler` and `policy RetryPolicy`; implement `Registry` with `mu sync.RWMutex` and `handlers map[string]entry`
- [ ] 3.4 Implement `Registry.Register(jobType string, handler JobHandler, policy RetryPolicy) error`: return error if type already registered
- [ ] 3.5 Implement `Registry.Get(jobType string) (entry, bool)` for internal lookup
- [ ] 3.6 Implement `Registry.ComputeRunAfter(policy RetryPolicy, attempt int) time.Time`: constant = `now + RetryDelay`; exponential = `now + RetryDelay * 2^attempt`

## 4. Worker Pool (`internal/jobs/worker.go`)

- [ ] 4.1 Define `Scheduler` struct: `store *Store`, `registry *Registry`, `instanceID string` (UUID generated at startup), `workerCount int`, `pollInterval time.Duration`, `claimedTimeout time.Duration`, `maxRetention time.Duration`, `ctx context.Context`, `cancel context.CancelFunc`, `wg sync.WaitGroup`
- [ ] 4.2 Implement `Scheduler.Start()`: spawn `workerCount` goroutines each calling `runWorker()`; spawn stale-recovery goroutine; spawn purge goroutine
- [ ] 4.3 Implement `runWorker(ctx)`: loop — call `store.Claim(instanceID, now)`; if nil sleep `pollInterval`; else dispatch to `handleJob(ctx, job)` before next iteration
- [ ] 4.4 Implement `handleJob(ctx, job)`: look up handler via `registry.Get(job.Type)`; call `handler.Handle(ctx, job)`; on nil error call `store.Complete`; on error: if `attempt < maxAttempts` call `store.Retry` with computed `run_after`; else call `store.Fail`
- [ ] 4.5 Implement stale-recovery goroutine: ticker at `claimedTimeout / 2`; call `store.RecoverStale(claimedTimeout)` on each tick; log count of reset jobs
- [ ] 4.6 Implement purge goroutine: ticker at `maxRetention / 4`; call `store.PurgeTerminal(maxRetention)` on each tick; log count of deleted jobs
- [ ] 4.7 Implement `Scheduler.Stop()`: cancel context; call `wg.Wait()` to drain all workers and goroutines before returning
- [ ] 4.8 Read `JOB_WORKER_COUNT` (default 4), `JOB_POLL_INTERVAL` (default 500ms), `JOB_CLAIMED_TIMEOUT` (default 5m), `JOB_MAX_RETENTION` (default 24h) from `internal/config/config.go`

## 5. API Handlers (`internal/jobs/api.go`)

- [ ] 5.1 Implement `GET /api/v1/jobs/:id` handler: call `store.GetByID`; return 200 with job JSON or 404 with `{"error":"job not found"}`
- [ ] 5.2 Implement `GET /api/v1/jobs` handler: parse `type`, `status`, `limit` (cap at 200, default 50), `offset` (default 0) query params; call `store.List`; return `{"jobs":[...],"total":N,"limit":N,"offset":N}`
- [ ] 5.3 Register both routes in `internal/server/api.go` under `/api/v1/jobs` with existing auth middleware

## 6. Browser-Start Handler Registration

- [ ] 6.1 Create `internal/browser/browser_start_handler.go`: implement `BrowserStartHandler` satisfying `JobHandler`; `Handle` unpacks `account_id` and `org_id` from `job.Payload`, acquires org semaphore slot, calls `DockerBrowserService.Start`, stores `cdp_port`/`vnc_port`/`container_id` result
- [ ] 6.2 Register `"browser_start"` handler in `cmd/scraper/main.go` after scheduler init: `jobRegistry.Register("browser_start", browserStartHandler, jobs.RetryPolicy{MaxAttempts: 3, BackoffStrategy: "exponential", RetryDelay: 10 * time.Second})`
- [ ] 6.3 Rewrite `internal/browser/scheduler.go` to a thin facade: `Submit(accountID, orgID int64)` marshals payload and calls `jobs.Submit("browser_start", fmt.Sprintf("account:%d", accountID), payload)`; delete `JobQueue` and worker pool logic
- [ ] 6.4 Delete `internal/browser/job_queue.go`; update all callers to use the new `scheduler.Submit` signature

## 7. HTTP Handler Updates

- [ ] 7.1 Update `POST /browser/start` handler: call `scheduler.Submit(accountID, orgID)` → returns `*jobs.Job`; respond HTTP 202 for new pending job, HTTP 200 for existing pending/running job; include `job_id`, `status`, `account_id` and optionally `cdp_port`/`vnc_port`/`container_id` when running
- [ ] 7.2 Update `POST /browser/stop` handler: after stopping container, call `store.Complete(jobID)` to mark the associated `scheduler_jobs` row completed and release the org semaphore slot
- [ ] 7.3 Remove any reference to `MAX_QUEUE_DEPTH` environment variable check from browser HTTP handlers

## 8. Wiring in `main.go`

- [ ] 8.1 Instantiate `jobs.NewRegistry()` and assign to package-level registry
- [ ] 8.2 Instantiate `jobs.NewScheduler(store, registry, cfg)` (generates instance UUID internally)
- [ ] 8.3 Call `scheduler.Start()` after all handlers are registered; call `scheduler.Stop()` in `defer` or shutdown hook
- [ ] 8.4 Pass `scheduler` to `Server` or browser handlers as needed to call `Submit` and `Complete`

## 9. Verification

- [ ] 9.1 `go build ./cmd/scraper/` passes with no new warnings
- [ ] 9.2 `go test ./internal/jobs/...` — unit tests: `Submit` idempotency, `Claim` single-winner (two goroutines racing), `RecoverStale` resets old running job, `PurgeTerminal` deletes expired terminal jobs, retry backoff calculation
- [ ] 9.3 Manual test: restart service while a job is pending — confirm job survives restart and is claimed by the new instance
- [ ] 9.4 Manual test: call `POST /browser/start` twice for the same account in quick succession — confirm second call returns HTTP 200 with the existing job ID, not a new one
- [ ] 9.5 Manual test: call `GET /api/v1/jobs?type=browser_start&status=pending` and confirm the queue is visible

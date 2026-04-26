## Context

The `docker-browser-service` change establishes a `BrowserServicer` interface backed by Docker. The HTTP handler currently calls `BrowserServicer.Start` synchronously per request. Under load (e.g., 1000 users hitting `POST /browser/start` simultaneously), this has two failure modes:

1. **Goroutine explosion**: Each HTTP request sits in a goroutine waiting for Docker to respond. Docker itself queues internally but all goroutines remain live, consuming stack memory.
2. **Cap enforcement race**: The concurrency check (`running < MAX`) in `DockerBrowserService` has a TOCTOU window — two goroutines can both pass the check before either registers a running container.

The scheduler closes both gaps by funneling all start requests through a bounded channel consumed by a fixed worker pool.

The system is single-node, in-process (no Redis, no external queue). Go channels + goroutines are sufficient and introduce zero new dependencies.

## Goals / Non-Goals

**Goals:**
- All `POST /browser/start` calls enqueue a job and return immediately (non-blocking).
- A fixed-size worker pool (size = `SCHEDULER_WORKER_COUNT`) pulls jobs and calls `BrowserServicer.Start`.
- Concurrency cap (`MAX_CONCURRENT_BROWSERS`) is enforced inside the scheduler, eliminating the TOCTOU race in the Docker service.
- Idempotent submission: same `account_id` while pending/running returns existing job ID.
- Bounded queue depth (`MAX_QUEUE_DEPTH`): overflow returns HTTP 429.
- Job state machine (`pending → scheduled → running → failed|completed`) is queryable via REST.
- `GET /browser/queue` exposes real-time depth and running count for ops dashboards.

**Non-Goals:**
- Persistent job storage (SQLite/Redis) — jobs are in-memory; lost on restart.
- Priority queuing — FIFO only.
- Distributed scheduling across multiple nodes.
- Retry logic on job failure — caller must resubmit.
- WebSocket push for job completion — polling only.

## Decisions

### 1. In-memory job store with `sync.Map` + a separate FIFO channel

**Decision**: Jobs are stored in a `sync.Map[jobID → *Job]`. The FIFO queue is a `chan *Job` with capacity `MAX_QUEUE_DEPTH`. Workers read from the channel; the store is used for status lookups and idempotency checks.

**Why**: Separating storage from the queue channel allows O(1) status lookup by job ID without scanning the channel. A buffered channel naturally enforces the depth cap (send fails when full).

**Alternative considered**: Single `[]Job` slice with mutex — rejected because a slice-based queue requires shifting elements on dequeue, and random-access lookup for idempotency would be O(n).

### 2. Worker pool size independent of concurrency cap

**Decision**: `SCHEDULER_WORKER_COUNT` (default: `MAX_CONCURRENT_BROWSERS`) controls how many goroutines pull from the queue. The concurrency cap is tracked as a separate semaphore (`chan struct{}` of capacity `MAX_CONCURRENT_BROWSERS`).

**Why**: The worker grabs a semaphore slot before calling Docker, holds it while the container is running, and releases it only on `Stop`. Workers without a slot block on the semaphore channel — they don't busy-wait and don't consume Docker API calls. This means worker count can be set higher than the cap for burst absorption without risking over-provisioning.

**Alternative considered**: Making worker count equal the concurrency cap and having each worker own one slot — simpler but means all workers are always blocked when at capacity, preventing them from doing accounting/cleanup work.

### 3. Semaphore released by `Stop`, not by job completion

**Decision**: The concurrency semaphore slot is acquired when a container starts running and released when `POST /browser/stop` is called (or container reconciliation removes it). Job status transitions to `completed` at that point.

**Why**: A browser container is a long-lived resource (the user browses, then stops). The slot must be held for the lifetime of the container, not just the start operation. Releasing on job-completion-of-start would immediately allow another container to start, defeating the cap.

**Alternative considered**: Track running containers directly via `DockerBrowserService.running` map length and poll it — rejected because it couples the scheduler to Docker internals and reintroduces the TOCTOU race.

### 4. Idempotency key = `account_id`

**Decision**: Before enqueuing, the scheduler scans the idempotency index (`sync.Map[accountID → jobID]`) for a pending or running job for the same account. If found, returns the existing job ID with its current status.

**Why**: An account can only have one browser anyway. Duplicate submissions are almost certainly from UI retries or network glitches.

**Alternative considered**: Caller-supplied idempotency key — more flexible but adds API surface and requires clients to generate and track keys. `account_id` is always known.

### 5. Async response with polling (no WebSocket push)

**Decision**: `POST /browser/start` returns HTTP 202 with `{ job_id, status, position }`. Client polls `GET /browser/jobs/:job_id` until status is `running` or `failed`.

**Why**: WebSocket push requires a hub per job and lifecycle cleanup. For a management UI with tens of accounts (not millions of users), polling every 1–2s is adequate and simpler.

**Alternative considered**: Long-poll or SSE — adds complexity without clear benefit for this use case.

## Risks / Trade-offs

- **Job loss on restart** → Mitigation: document that in-flight jobs are lost on restart; callers should re-check `GET /browser/:id/status` and resubmit if needed. A future persistence layer can address this.
- **Queue depth too small causes spurious 429s** → Mitigation: `MAX_QUEUE_DEPTH` defaults to 500 (far above normal account count); document tuning guidance.
- **Semaphore never released if Stop is not called** → Mitigation: `BrowserService` reconciliation on startup removes orphan containers and triggers slot release; add a watchdog goroutine that checks for containers running > configurable max lifetime.
- **`sync.Map` idempotency index grows unbounded** → Mitigation: entries are removed when job reaches terminal state (`failed` or `completed`). Map size is bounded by number of accounts.
- **Breaking API change to `POST /browser/start`** → Mitigation: frontend `app.js` is the only consumer; update it in the same PR. No external API clients exist yet.

## Migration Plan

1. Implement `job_queue.go` and `scheduler.go` in `internal/browser/`.
2. Update `browser_handlers.go`: `POST /browser/start` calls `Scheduler.Submit()` instead of `BrowserServicer.Start()`.
3. Add `GET /browser/jobs/:job_id` and `GET /browser/queue` handlers.
4. Update `app.js`: after submit, poll job status; show "queued (position N)" pill state.
5. Update `cmd/scraper/main.go` to instantiate and start the scheduler.
6. Deploy: no data migration required (jobs are ephemeral). Stop → redeploy → start. In-flight containers survive (Docker doesn't stop with the Go process); reconciliation on restart re-registers them.
7. Rollback: revert `browser_handlers.go` to call `BrowserServicer.Start` directly — scheduler is additive and can be bypassed.

## Open Questions

- Should `GET /browser/jobs/:job_id` be authenticated (JWT required) or open to the agent extension? Currently all `/browser/*` routes require JWT.
- What should the default `MAX_QUEUE_DEPTH` be? 500 is proposed; may need tuning based on actual account count at production.
- Should failed jobs auto-retry once before transitioning to `failed`? Current design does not retry.

> **Lifecycle status (2026-07-21 spec IA reconciliation):** proposal only — nothing under `openspec/` is current runtime authority (per `AGENTS.md`/`CLAUDE.md`; the runtime authority is `specs/domains/platform-foundation/features/runtime-topology/technical.md`). NOT IMPLEMENTED (depends on the unimplemented docker-browser-service). The durable queue piece it assumes exists today as `internal/jobs`; the container-start scheduling itself has no realized counterpart.

## Why

The `docker-browser-service` change (previous change) routes `POST /browser/start` directly to Docker container creation, meaning 1000 simultaneous API calls would attempt to spin up 1000 containers at once — overwhelming the Docker daemon, exhausting ports, and crashing the node. A scheduler layer that queues and paces container start requests is required before the service can operate safely under real user load.

## What Changes

- Introduce a `BrowserScheduler` struct that sits between the HTTP handler and `BrowserServicer`.
- All `POST /browser/start` requests are submitted as jobs to a FIFO queue; the scheduler drains the queue up to `MAX_CONCURRENT_BROWSERS` at a time.
- Job lifecycle: `pending → scheduled → running → failed | completed`.
- **BREAKING**: `POST /browser/start` no longer returns container info synchronously; it returns `{ "job_id", "status": "pending"|"running", "position": N }`.
- New endpoints:
  - `GET /browser/jobs/:job_id` — poll job status and result.
  - `GET /browser/queue` — current queue depth and running count (ops visibility).
- Idempotent submission: a second `POST /browser/start` for the same `account_id` while a job is already pending/running returns the existing job instead of enqueuing a duplicate.
- When queue is full (configurable `MAX_QUEUE_DEPTH`), return HTTP 429 with queue position context.
- Backpressure: worker goroutines pull from the queue; no goroutine is spawned per request.

## Capabilities

### New Capabilities

- `browser-job-queue`: FIFO in-memory job queue with bounded depth, idempotent submission, and backpressure (429 on overflow).
- `browser-scheduler`: Worker pool that drains the job queue and invokes `BrowserServicer.Start` up to the concurrency cap; tracks job state transitions.
- `browser-job-status-api`: REST endpoints to query individual job status/result and overall queue depth.

### Modified Capabilities

- `browser-container-lifecycle`: `POST /browser/start` response shape changes — now returns a job reference instead of immediate container info. **Requires delta spec.**

## Impact

- **Code**: New `internal/browser/scheduler.go` and `internal/browser/job_queue.go`; `internal/server/browser_handlers.go` updated to go through scheduler; new handler file for job-status endpoints.
- **APIs**: `POST /browser/start` response shape changes (breaking); two new endpoints added.
- **Frontend**: `app.js` `browserStartAccount()` must poll `GET /browser/jobs/:job_id` after submitting start; UI needs a "queued" state in the status pill.
- **Config**: New env vars `MAX_QUEUE_DEPTH`, `SCHEDULER_WORKER_COUNT`.
- **No new external dependencies** — uses Go standard library channels and goroutines only.

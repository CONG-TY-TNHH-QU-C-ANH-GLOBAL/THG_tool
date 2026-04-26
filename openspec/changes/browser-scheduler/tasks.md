## 1. Config and Environment

- [ ] 1.1 Add `MAX_QUEUE_DEPTH` and `SCHEDULER_WORKER_COUNT` to `internal/config/config.go` with defaults (500 and `MAX_CONCURRENT_BROWSERS` respectively)
- [ ] 1.2 Add the two new env vars to `.env.example` with documented defaults

## 2. Job Model

- [ ] 2.1 Create `internal/browser/job.go` — define `JobStatus` type and constants (`pending`, `scheduled`, `running`, `failed`, `completed`)
- [ ] 2.2 Define `Job` struct with fields: `ID`, `AccountID`, `Status`, `Position`, `CDPPORT`, `VNCPort`, `ContainerID`, `Error`, `CreatedAt`, `StartedAt`, `FailedAt`
- [ ] 2.3 Implement `Job.IsTerminal() bool` helper returning true for `failed` and `completed`

## 3. Job Queue

- [ ] 3.1 Create `internal/browser/job_queue.go` — `JobQueue` struct with `queue chan *Job`, `store sync.Map` (jobID→Job), `idempotency sync.Map` (accountID→jobID), and semaphore `chan struct{}`
- [ ] 3.2 Implement `NewJobQueue(maxDepth, maxConcurrent int) *JobQueue`
- [ ] 3.3 Implement `Submit(accountID int64) (*Job, bool, error)` — idempotency check, cap check, enqueue, return (job, isNew, err); HTTP 429 case returns error
- [ ] 3.4 Implement `Transition(jobID string, newStatus JobStatus, update func(*Job)) error` — enforce valid state machine transitions, clean idempotency index on terminal states
- [ ] 3.5 Implement `Get(jobID string) (*Job, bool)` — lookup from store
- [ ] 3.6 Implement `QueueDepth() int` and `RunningCount() int` for ops endpoint
- [ ] 3.7 Implement `QueuePosition(jobID string) int` — count pending jobs ahead in FIFO order (approximate via atomic counter or scan)
- [ ] 3.8 Write unit tests: successful submit, duplicate submit (pending), duplicate submit (running), resubmit after failure, queue full 429, state transitions (valid and invalid), terminal cleanup

## 4. Scheduler Worker Pool

- [ ] 4.1 Create `internal/browser/scheduler.go` — `Scheduler` struct with `queue *JobQueue`, `browserSvc BrowserServicer`, `workers int`, `ctx context.Context`, `cancel context.CancelFunc`
- [ ] 4.2 Implement `NewScheduler(queue *JobQueue, svc BrowserServicer, workers int) *Scheduler`
- [ ] 4.3 Implement `Start()` — launch `workers` goroutines each running `workerLoop()`
- [ ] 4.4 Implement `workerLoop()` — range over `queue.queue` channel; acquire semaphore slot; transition job to `scheduled`; call `BrowserServicer.Start`; transition to `running` or `failed`; release semaphore on failure; on success hold slot until `Stop` releases it
- [ ] 4.5 Implement `Stop(ctx context.Context, accountID int64) error` — delegate to `BrowserServicer.Stop`, release semaphore slot, transition job to `completed`
- [ ] 4.6 Implement `Shutdown(ctx context.Context)` — cancel context, drain pending jobs to `failed` with reason "service shutting down", wait for workers to exit
- [ ] 4.7 Write unit tests: worker picks up job, semaphore blocks at cap, slot released on failure, shutdown drains pending, duplicate stop is a no-op

## 5. REST Handler Updates

- [ ] 5.1 Update `POST /browser/start` handler in `internal/server/browser_handlers.go` to call `Scheduler.Submit()` instead of `BrowserServicer.Start()`; return HTTP 202 with job reference or HTTP 200 for idempotent running job
- [ ] 5.2 Update `POST /browser/stop` handler to call `Scheduler.Stop()` instead of `BrowserServicer.Stop()` so semaphore slot is released correctly
- [ ] 5.3 Add `GET /browser/jobs/:job_id` handler — call `JobQueue.Get()`, format response per job state (include `position` for pending, container info for running, error for failed)
- [ ] 5.4 Add `GET /browser/queue` handler — call `JobQueue.QueueDepth()` and `RunningCount()`, return depth/running/capacity/queue_depth JSON
- [ ] 5.5 Register the two new routes in `internal/server/api.go`

## 6. Wiring in main.go

- [ ] 6.1 In `cmd/scraper/main.go`, instantiate `JobQueue` after `PortRegistry` and `DockerBrowserService`
- [ ] 6.2 Instantiate `Scheduler` with `JobQueue`, `BrowserService`, and `SCHEDULER_WORKER_COUNT`
- [ ] 6.3 Call `Scheduler.Start()` before the HTTP server starts
- [ ] 6.4 Add `Scheduler.Shutdown(ctx)` to the deferred/shutdown path

## 7. Frontend Updates

- [ ] 7.1 Update `browserStartAccount()` in `app.js` to handle HTTP 202 response — extract `job_id` and start polling `GET /browser/jobs/:job_id` every 1.5s
- [ ] 7.2 Add `"queued"` state to the account status pill (distinct color from `"running"` and `"stopped"`)
- [ ] 7.3 Show queue position in the pill label when `status === "pending"` (e.g., "Queued #3")
- [ ] 7.4 Transition pill to `"running"` when poll returns `status === "running"`; stop polling
- [ ] 7.5 Show error toast when poll returns `status === "failed"` with the error message; stop polling

## 8. Verification

- [ ] 8.1 `go build ./cmd/scraper/` passes with no new warnings
- [ ] 8.2 Run unit tests: `go test ./internal/browser/...` — all new tests pass
- [ ] 8.3 Manual load test: submit 10 simultaneous `POST /browser/start` requests with `MAX_CONCURRENT_BROWSERS=3`; verify only 3 containers start, rest queue; verify `GET /browser/queue` shows correct pending count
- [ ] 8.4 Verify idempotency: submit `POST /browser/start` twice for same account while first is pending; confirm only one job created
- [ ] 8.5 Verify shutdown: send SIGTERM while jobs are pending; confirm pending jobs transition to `failed` and no new containers are started

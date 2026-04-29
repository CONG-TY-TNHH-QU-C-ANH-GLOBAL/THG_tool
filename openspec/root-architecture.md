# Root Architecture Specification
## Multi-Tenant Browser Orchestration System

> **Governance document only.** All modules described here are implemented. This spec defines system-wide boundaries, ownership rules, and invariants. No new modules are introduced.

---

## 0. Production Open Crawler Boundary

THG production crawling is prompt-scoped, not a fixed configured-group scraper.
Telegram and dashboard chat are ingress transports into the same AI agent/action
pipeline. The agent must turn explicit user intent into durable jobs such as
`facebook_crawl`, `web_crawl`, or `lead_gen`.

System rules:

- A crawler job needs a concrete target URL, search query, or campaign context.
- Broad "scan all" requests must ask for a target instead of scanning every saved group.
- Workers attach to the selected account's visible workspace Chrome session.
- Classification against current business context runs before candidates become leads.
- Mock frontend data, embedded legacy HTML, hidden browser pools, and duplicate API services are prohibited in production.

---

## 1. Global Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  INGRESS PLANE                                                              │
│                                                                             │
│   Telegram Bot ──────────────────┐                                         │
│   HTTP Client ──→ Gofiber API ───┤                                         │
│                    (OrgScope     │                                         │
│                     middleware)  │                                         │
└──────────────────────────────────┼──────────────────────────────────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │       CONTROL PLANE          │
                    │  • OrgSemaphoreRegistry      │
                    │  • API Key auth (thg_ prefix)│
                    │  • Plan tier quota checks    │
                    │  • Superadmin routes         │
                    └──────────────┬──────────────┘
                                   │
          ┌────────────────────────┼────────────────────────┐
          │                        │                        │
          ▼                        ▼                        ▼
┌─────────────────┐    ┌──────────────────────┐   ┌─────────────────┐
│  WARM POOL      │    │  JOB SCHEDULER       │   │  STREAMING      │
│                 │    │  (scheduler_jobs)    │   │  LAYER          │
│  Per-account    │    │                      │   │                 │
│  pre-warmed     │ ─→ │  • Submit (idempotent│   │  • VNCProvider  │
│  Docker slots   │    │    INSERT OR IGNORE) │   │  • WebRTCStub   │
│                 │    │  • Worker pool (N=4) │   │  • SessionMgr   │
│  HIT → HTTP 200 │    │  • Atomic claim via  │   │                 │
│  MISS → submit  │    │    subquery UPDATE   │   │  Viewer tokens  │
│  to scheduler   │    │  • Stale recovery    │   │  per session    │
└────────┬────────┘    └──────────┬───────────┘   └────────┬────────┘
         │                        │                         │
         │              ┌─────────▼─────────┐              │
         │              │  BROWSER RUNTIME  │              │
         └─────────────→│  MANAGER          │←─────────────┘
                        │  (browser_containers)            │
                        │                   │
                        │  • FSM (8 states) │
                        │  • HealthProbe    │
                        │  • RestartPolicy  │
                        │  • Reconciliation │
                        └─────────┬─────────┘
                                  │
                        ┌─────────▼─────────┐
                        │  PORT REGISTRY    │
                        │  (lease-based)    │
                        │                  │
                        │  • Atomic bitset │
                        │  • TTL leases    │
                        │  • MemBackend /  │
                        │    RedisBackend  │
                        └─────────┬─────────┘
                                  │
                        ┌─────────▼─────────┐
                        │  DOCKER SERVICE   │
                        │  (DockerBrowser   │
                        │   Service)        │
                        │                  │
                        │  • Create         │
                        │  • Start          │
                        │  • Stop           │
                        │  • List/Inspect   │
                        └─────────┬─────────┘
                                  │
                        ┌─────────▼─────────┐
                        │  SQLITE DATABASE  │
                        │  (single file)    │
                        │                  │
                        │  scheduler_jobs   │
                        │  browser_containers│
                        │  organizations    │
                        │  accounts         │
                        │  api_keys         │
                        └───────────────────┘
```

### Layer call direction rules

```
INGRESS → CONTROL PLANE → WARM POOL → SCHEDULER → RUNTIME MANAGER → PORT REGISTRY → DOCKER SERVICE
                                           ↓
                                    STREAMING LAYER (lateral, not vertical)
```

**No upward calls.** Lower layers never call higher layers. The sole exception is the `RestartPolicy` component inside Runtime Manager, which calls `jobs.Submit` (upward to Scheduler) — this is the only permitted upward call and is explicitly documented as an exception in §5 (Anti-Patterns).

---

## 2. Single Source of Truth Rules

### 2.1 Job State — `scheduler_jobs` table owns all job state

| Owned by | `scheduler_jobs` (SQLite table) |
|---|---|
| Authority | `internal/jobs/store.go` |
| States | `pending`, `running`, `failed`, `completed` |
| Who may write | Job Scheduler package only (`internal/jobs/`) |
| Who may read | Scheduler workers, REST API handlers, Runtime Manager's browser_start handler |

**Rule**: No other system writes to `scheduler_jobs`. The Runtime Manager does not update job rows directly — it calls `jobs.Complete(jobID)` or `jobs.Fail(jobID)` exclusively via the `internal/jobs` package API, never via raw SQL from outside that package.

**Rule**: Job state is not derived from Docker state. If Docker says a container is running but the job says `failed`, the job record is authoritative for the job lifecycle. Container state is a separate concern (§2.2).

### 2.2 Container Lifecycle State — `browser_containers` table owns all container state

| Owned by | `browser_containers` (SQLite table) |
|---|---|
| Authority | `internal/browser/runtime_manager.go` |
| States | `pending`, `creating`, `starting`, `running`, `unhealthy`, `stopping`, `stopped`, `removed` |
| Who may write | `BrowserRuntimeManager` only, via `TransitionBrowserContainer` |
| Who may read | Runtime Manager, REST `/browser/:id/runtime`, startup reconciliation |

**Rule**: No other system writes to `browser_containers`. The Job Scheduler does not transition container states. The Warm Pool does not write `browser_containers` rows directly — it calls `BrowserRuntimeManager.StartContainer`, which owns the write.

**Rule**: On state conflict between `browser_containers` and Docker, Docker wins for liveness, but `browser_containers` wins for intent. Specifically: if Docker says `exited` and the DB says `running`, the DB is corrected to `stopped` during reconciliation. If Docker says `running` and the DB says `removed`, the container is treated as an orphan and re-registered.

### 2.3 Restart Responsibility — Runtime Manager decides, Scheduler executes

| Decision owner | `BrowserRuntimeManager` + `RestartPolicy` |
|---|---|
| Execution owner | `internal/jobs/` (via `jobs.Submit("browser_start", ...)`) |

**Rule**: The Runtime Manager is the only system that evaluates the restart policy. It calls `jobs.Submit` to enqueue a new start. The Scheduler does not know why a job was submitted — it treats restart-triggered submissions identically to user-initiated ones. The Scheduler never triggers restarts itself.

**Rule**: `restart_count` in `browser_containers` is incremented atomically with the state transition that triggers the restart. It is never incremented by the Scheduler.

**Rule**: Intentional stops (via `POST /browser/stop`) do not increment `restart_count` and do not evaluate the restart policy. This is enforced by the Runtime Manager's `StopContainer` code path, which transitions to `stopped → removed` without calling `RestartPolicy.ShouldRestart`.

### 2.4 Port Allocation Authority — Port Registry is the sole allocator

| Owned by | `PortRegistry` (in-memory bitset + lease map) |
|---|---|
| Authority | `internal/browser/port_registry.go` |
| Allocation | Atomic CAS on `[]uint64` bitset; returns `leaseID` |
| Release | `Release(leaseID)` only — never by port number |
| Who may allocate | `DockerBrowserService.Start` only |
| Who may release | `DockerBrowserService.Stop` + stale lease reaper ticker |

**Rule**: No other system calls `PortRegistry.Acquire` or `PortRegistry.Release`. The Runtime Manager does not directly touch the Port Registry — it delegates to `DockerBrowserService`, which owns the acquire/release calls.

**Rule**: Port numbers are ephemeral. They are stored in the job payload and in the `browser_containers` row for observability, but the Port Registry (not the DB) is the authority for whether a port is currently leased.

### 2.5 Org Concurrency — OrgSemaphoreRegistry is ephemeral, not authoritative

| Owned by | `OrgSemaphoreRegistry` (in-memory `map[orgID]chan struct{}`) |
|---|---|
| Authority | In-memory only; not persisted |
| Populated from | `org.EffectiveQuota().MaxConcurrentBrowsers` at runtime |
| Who may acquire | Runtime Manager's `browser_start` handler, via worker goroutine |
| Who may release | Runtime Manager's `StopContainer` and failure paths |

**Rule**: The OrgSemaphoreRegistry is an ephemeral concurrency guard, not a source of truth. On process restart it is empty. It is repopulated by the Scheduler as workers claim and execute jobs. In-flight semaphore slots that existed before a crash are recovered by the stale job recovery path (§6.3).

---

## 3. Execution Flow

### 3.1 Happy path: `POST /browser/start`

```
[1] HTTP handler receives POST /browser/start {account_id}
      │
[2] OrgScope middleware validates bearer token (thg_ API key or JWT)
    → injects orgID, role, userID into Fiber context
      │
[3] Handler fetches account.org_id; compares to context orgID
    → mismatch: HTTP 403, flow ends
      │
[4] WarmBrowserPool.Claim(accountID)
      ├─ HIT ──────────────────────────────────────────────────────────────────┐
      │   BrowserRuntimeManager.StartContainer() (sync, container pre-running) │
      │   → update browser_containers: state='running'                         │
      │   → ContainerHealthProbe.Start(accountID, cdpPort)                     │
      │   → return HTTP 200 {status,cdp_port,vnc_port,container_id}            │
      │   ────────────────────────────────────────────────────────── END        │
      │                                                                         │
      └─ MISS ──────────────────────────────────────────────────────────────────┤
          jobs.Submit("browser_start", "account:<id>", payload)                 │
          → INSERT OR IGNORE INTO scheduler_jobs                                │
          → SELECT by (type, idempotency_key) — returns existing or new row     │
          → return HTTP 202 {job_id, status, account_id}                        │
          ─────────────────────────────────────────────────────────── END       │
                                                                                │
[5] Worker goroutine (async, in background):                                    │
      Claim UPDATE: scheduler_jobs pending → running (atomic)                   │
      → BrowserStartHandler.Handle(ctx, job)                                    │
        → OrgSemaphoreRegistry.Acquire(orgID, maxConcurrent) — blocks if full  │
        → BrowserRuntimeManager.StartContainer(accountID, orgID)                │
          → store.UpsertBrowserContainer(state='pending')                       │
          → Transition: pending → creating                                      │
          → PortRegistry.Acquire() → (cdpPort, vncPort, leaseID)               │
          → DockerBrowserService.Start() → Docker API create + start            │
          → Transition: creating → starting → running                           │
          → ContainerHealthProbe.Start(accountID, cdpPort)                      │
          → store payload (cdp_port, vnc_port, container_id) in job             │
        → jobs.Complete(jobID): scheduler_jobs.status = 'completed'            │
        ← return nil                                                            │
      ← handler returns nil: job marked completed                               │
                                                                                │
      ON HANDLER ERROR:                                                          │
        → OrgSemaphoreRegistry.Release(orgID)                                  │
        → PortRegistry.Release(leaseID)                                         │
        → Transition browser_containers to 'stopped'                            │
        → if attempt < maxAttempts: scheduler_jobs back to 'pending' + runAfter │
        → if attempt >= maxAttempts: scheduler_jobs to 'failed'                 │
```

### 3.2 Happy path: `POST /browser/stop`

```
[1] HTTP handler receives POST /browser/stop {account_id}
      │
[2] OrgScope middleware + org ownership check (same as start)
    → mismatch: HTTP 403
      │
[3] BrowserRuntimeManager.StopContainer(accountID, orgID)
      → store.GetBrowserContainer(accountID)
      → state not in {running, unhealthy}: HTTP 404, flow ends
      │
[4] Transition: running → stopping
      → ContainerHealthProbe.Stop(accountID) — cancel probe goroutine
      → DockerBrowserService.Stop() → Docker API stop + remove
      → Transition: stopping → stopped → removed
      │
[5] jobs.Complete(jobID) — mark associated scheduler_jobs row completed
      → OrgSemaphoreRegistry.Release(orgID)
      → PortRegistry.Release(leaseID)
      │
[6] RestartPolicy NOT evaluated (intentional stop path)
      → restart_count NOT incremented
      │
[7] HTTP 200
```

### 3.3 Health failure path

```
ContainerHealthProbe goroutine detects N consecutive failures
      │
→ manager.MarkUnhealthy(accountID)
    → Transition: running → unhealthy
    → RestartPolicy.ShouldRestart(restart_count)?
        │
        ├─ NO (never / limit exhausted):
        │   → Transition: unhealthy → stopping → stopped
        │   → ContainerHealthProbe.Stop()
        │   → DockerBrowserService.Stop()
        │   → PortRegistry.Release(leaseID)
        │   → OrgSemaphoreRegistry.Release(orgID)
        │   → Log warning: account_id, final restart_count
        │
        └─ YES:
            → store.IncrementRestartCount(accountID)
            → Transition: unhealthy → stopping → stopped
            → ContainerHealthProbe.Stop()
            → DockerBrowserService.Stop()
            → PortRegistry.Release(leaseID)
            → OrgSemaphoreRegistry.Release(orgID)
            → jobs.Submit("browser_start", "account:<id>", payload)
              run_after = now  (immediate requeue)
            → Transition: stopped → pending
            (Scheduler worker picks it up on next poll cycle)
```

### 3.4 Stale job recovery path (crash recovery)

```
Scheduler stale recovery goroutine (ticks every claimedTimeout/2):
      │
→ UPDATE scheduler_jobs SET status='pending', claimed_by=NULL, claimed_at=NULL
  WHERE status='running' AND claimed_at < (now - claimedTimeout)
      │
→ Affected rows are re-claimed by the next available worker
      │
NOTE: browser_containers rows for those jobs may be in 'creating' or 'starting'
state from before the crash. Startup reconciliation handles them:
  → Docker container missing → browser_containers → 'removed'
  → Policy evaluated → new job submitted if ShouldRestart()
The re-claimed scheduler_jobs row starts a fresh container lifecycle.
```

---

## 4. Conflict Resolution Rules

### 4.1 scheduler_jobs vs browser_containers disagree on running state

**Scenario**: `scheduler_jobs.status = 'running'` but `browser_containers.state = 'stopped'`

**Resolution**: `browser_containers` wins. The Runtime Manager's reconciliation path detects `stopped` and evaluates the restart policy. The stale job recovery path resets the `scheduler_jobs` row to `pending` (via `claimed_at` timeout). A new worker claims it and invokes a fresh container start. The old `running` scheduler_jobs row is an artifact of the crashed worker — the timeout corrects it automatically.

**Rule**: The Scheduler does not read `browser_containers`. The Runtime Manager does not read `scheduler_jobs` directly. Conflicts are resolved by timeout (scheduler side) and reconciliation (runtime side) independently.

### 4.2 Docker state vs browser_containers disagree

**Resolution**: Docker is authoritative for physical container existence. `browser_containers` is authoritative for intended state. Startup reconciliation applies Docker's truth to `browser_containers`:

| Docker state | browser_containers state | Resolution |
|---|---|---|
| Container missing | `creating/starting/running/unhealthy/stopping` | DB → `removed`; eval restart policy |
| Container `running` | No row | Insert row with `state='running'`; start health probe |
| Container `exited` | `running` | DB → `stopped`; eval restart policy |
| Container `running` | `running` | No action; ensure health probe is active |
| Container missing | `stopped/removed` | No action (expected terminal state) |

### 4.3 Two workers race to claim the same job

**Resolution**: SQLite's exclusive write lock serializes the claim UPDATE. The subquery `WHERE id=(SELECT id … LIMIT 1)` is evaluated inside a single exclusive write transaction. One worker receives the row in `RETURNING *`; the other receives zero rows and returns to polling. No application-level lock or coordination is needed.

### 4.4 Two goroutines race to transition the same container state

**Resolution**: `UPDATE browser_containers SET state=<new> WHERE account_id=? AND state=<expected>` returns 0 rows if the current state is not `<expected>`. The losing goroutine discards its transition attempt (no error propagated to callers — logged at DEBUG). The winning goroutine proceeds. This is the only mechanism; no mutex wraps FSM transitions.

### 4.5 OrgSemaphoreRegistry slot count vs actual running containers disagree

**Resolution**: The semaphore is ephemeral; on divergence after a crash, the stale job recovery path resets `running` jobs to `pending`. Workers re-acquire semaphore slots as they re-claim jobs. Slots are never leaked permanently because `Release` is called in all `StopContainer` and failure paths (including the `handle error` branch in §3.1).

If the semaphore count exceeds the actual container count (leaked slots), the quota enforcement is looser than configured until the process restarts. This is acceptable: the semaphore is a soft cap, not a billing boundary.

---

## 5. Anti-Patterns (MUST NOT EXIST)

### AP-1: Duplicate job queue systems

**Prohibited**: Any in-memory `chan *Job`, `sync.Map` job store, or unbuffered queue outside `scheduler_jobs`.

**Why**: The `browser-scheduler` change's `chan *Job` queue was explicitly deleted as part of the `idempotent-job-scheduler` change. All job submission goes through `jobs.Submit`. If any code submits work via a Go channel instead of `jobs.Submit`, it bypasses durability, idempotency, and retry.

**Detection**: Grep for `chan \*Job`, `chan Job`, `sync.Map` keyed by account ID in `internal/browser/`.

### AP-2: Runtime-triggered job creation outside the restart policy

**Prohibited**: Any component other than `BrowserRuntimeManager.MarkUnhealthy` and startup reconciliation calling `jobs.Submit("browser_start", ...)`.

**Why**: If the health probe, the DockerBrowserService, or any other subsystem submits jobs directly, the restart count tracking in `browser_containers` is bypassed, making `on-failure:<N>` enforcement impossible and potentially causing infinite restart loops.

**Permitted callers of `jobs.Submit("browser_start", ...)`**:
1. `POST /browser/start` HTTP handler (user-initiated)
2. `BrowserRuntimeManager.MarkUnhealthy` (health failure path)
3. Startup reconciliation (orphan/ghost recovery)

### AP-3: Scheduler-triggered container operations

**Prohibited**: Any code in `internal/jobs/` calling `DockerBrowserService`, `BrowserRuntimeManager`, or `ContainerHealthProbe` directly.

**Why**: The Scheduler is a generic execution engine. It invokes a registered `JobHandler`. The handler (`BrowserStartHandler`) owns the Docker interaction. If the Scheduler gains direct Docker knowledge, the abstraction boundary collapses and the system cannot reuse the Scheduler for non-browser job types (scrape, inbox, etc.).

**Rule**: `internal/jobs/` has zero imports from `internal/browser/` or `internal/docker/`.

### AP-4: Dual port ownership

**Prohibited**: Any code calling `net.Listen` to test port availability, hard-coding ports, or allocating ports outside `PortRegistry.Acquire`.

**Why**: The Port Registry's CAS-based bitset is the only mechanism that prevents double allocation under concurrency. A direct `net.Listen` test introduces a TOCTOU race: the port could be acquired by another goroutine between the test and the container creation. Hard-coded ports conflict when multiple accounts share a host.

**Rule**: Every port number used by a browser container must have a corresponding active `leaseID` in the Port Registry. Port numbers stored in `browser_containers` and `scheduler_jobs` payload are for observability only — they do not constitute an allocation.

### AP-5: Direct `browser_containers` writes from outside Runtime Manager

**Prohibited**: Any code outside `internal/browser/runtime_manager.go` executing `UPDATE browser_containers SET state=...`.

**Why**: The `WHERE state=<expected>` transition guard is the sole FSM enforcement mechanism. If another package writes `state` directly, it bypasses the guard and can produce illegal transitions (e.g., `removed → running`) that break reconciliation and health probe lifecycle.

### AP-6: Synchronous container start in HTTP handler

**Prohibited**: Any HTTP handler blocking on `DockerBrowserService.Start` in the request goroutine.

**Why**: Container start takes 2–4 seconds cold. At 100 concurrent requests, this exhausts the Gofiber goroutine pool. All container starts go through either the Warm Pool (pre-started, synchronous claim only) or the Job Scheduler (async, returns job ID immediately).

---

## 6. System Invariants

### 6.1 Idempotency guarantees

**Invariant I-1**: For any `(type, idempotency_key)` pair, at most one non-terminal `scheduler_jobs` row exists at any time. Guaranteed by `UNIQUE(type, idempotency_key)` + `INSERT OR IGNORE`.

**Invariant I-2**: For any `account_id`, at most one `browser_containers` row exists. Guaranteed by `account_id INTEGER PRIMARY KEY`.

**Invariant I-3**: Calling `POST /browser/start` for the same account N times while a job is `pending` or `running` creates exactly one container. The second through Nth calls return the existing job row. Guaranteed by I-1 + the unconditional fetch-after-insert in `jobs.Submit`.

**Invariant I-4**: `POST /browser/stop` is idempotent: calling it when no container is running returns HTTP 404 without side effects.

### 6.2 Restart safety rules

**Invariant R-1**: `restart_count` is monotonically non-decreasing for the lifetime of a `browser_containers` row. It is never decremented.

**Invariant R-2**: The restart policy is evaluated exactly once per unintentional stop event. It is not re-evaluated by the Scheduler, health probe, or reconciliation independently — only by `BrowserRuntimeManager.MarkUnhealthy` and the reconciliation path.

**Invariant R-3**: An `on-failure:<N>` policy with `restart_count >= N` results in a terminal `stopped` state with no further job submissions. There is no mechanism that bypasses this check.

**Invariant R-4**: The `always` policy does not cause busy-loop restarts. Between each restart, the submitted job must be claimed by a worker and the container must be started (at minimum 2–4s) before the next failure can trigger another restart evaluation.

### 6.3 Crash recovery behavior

**Invariant C-1**: All `scheduler_jobs` rows survive process restart. Jobs in `running` state at crash time are reset to `pending` by the stale job recovery goroutine within `claimedTimeout` (default 5 minutes) of the next process startup.

**Invariant C-2**: All `browser_containers` rows survive process restart. The startup reconciliation pass corrects any state that diverged from Docker reality during the crash window.

**Invariant C-3**: Port leases do NOT survive process restart (in-memory MemoryBackend). After restart, `DockerBrowserService.List` is called during reconciliation. Running containers have their ports re-registered in the Port Registry. This reconciliation is the responsibility of startup reconciliation in `BrowserRuntimeManager.Start`.

**Invariant C-4**: The OrgSemaphoreRegistry does NOT survive process restart. Semaphore slots are re-acquired naturally as the stale job recovery path resets crashed `running` jobs to `pending` and workers re-claim and re-execute them.

**Invariant C-5**: A crash during the `creating → starting` FSM window leaves a `browser_containers` row in `creating` or `starting` state. Startup reconciliation detects the corresponding Docker container:
- Container exists and is running → advance row to `running`, start health probe.
- Container does not exist → advance row to `removed`, evaluate restart policy.

### 6.4 Org isolation invariants

**Invariant O-1**: No HTTP handler accesses another org's accounts, jobs, or containers. The `OrgScope` middleware injects `orgID`; all store queries are parameterized on `orgID`. `orgID=0` is the superadmin sentinel, bypassing org checks (not restricting to nothing).

**Invariant O-2**: Per-org concurrency caps (`OrgSemaphoreRegistry`) are independent. Org A at its cap does not block Org B from acquiring a slot. Backed by separate `chan struct{}` channels keyed by `orgID`.

**Invariant O-3**: A container started for Org A's account cannot be stopped by Org B's API key. The org ownership check in `StopContainer` enforces this at the DB level (fetch `account.org_id`, compare to caller's `orgID`).

---

## 7. Module Ownership Summary

| Concern | Owning module | Backing store |
|---|---|---|
| Job submission + idempotency | `internal/jobs` (Scheduler) | `scheduler_jobs` (SQLite) |
| Job execution + retry | `internal/jobs` (Worker pool) | `scheduler_jobs` (SQLite) |
| Container FSM + health | `internal/browser` (RuntimeManager) | `browser_containers` (SQLite) |
| Container restart decision | `internal/browser` (RestartPolicy) | `browser_containers.restart_count` |
| Port allocation | `internal/browser` (PortRegistry) | In-memory bitset + lease map |
| Org concurrency cap | `internal/browser` (OrgSemaphoreRegistry) | In-memory `map[orgID]chan struct{}` |
| Warm container supply | `internal/browser` (WarmPool) | In-memory `map[accountID]*WarmSlot` |
| Streaming sessions | `internal/browser` (StreamingProvider) | In-memory session map + JWT |
| Auth + org scoping | `internal/server` (OrgScope middleware) | `organizations`, `api_keys` (SQLite) |
| Plan quotas | `internal/controlplane` (EffectiveQuota) | `organizations.plan_tier` (SQLite) |

---

## 8. Dependency Graph (import rules)

```
internal/jobs          → internal/store, internal/config
internal/browser       → internal/jobs (Submit only), internal/store, internal/config
internal/server        → internal/browser, internal/jobs, internal/store, internal/auth
internal/controlplane  → internal/store, internal/config
internal/store         → (no internal imports)
internal/config        → (no internal imports)
```

**Prohibited imports:**
- `internal/jobs` must NOT import `internal/browser`
- `internal/store` must NOT import any other `internal/` package
- `internal/config` must NOT import any other `internal/` package
- Any package must NOT import `cmd/scraper` (binary entry point only)

## Why

The `browser-scheduler` change queues Docker container starts and prevents overload, but each start still takes 2–4s (Xvfb + VNC + Chrome initialization inside the container). For a Facebook automation tool where staff frequently switch accounts or open sessions, this latency is noticeable on every login. Pre-warming containers per account eliminates cold-start delay by having a ready container waiting before the request arrives.

## What Changes

- Introduce a `WarmPool` component that, on startup and after each assignment, proactively starts Docker Chrome containers for known active accounts and holds them idle until claimed.
- `POST /browser/start` checks the warm pool first; if a ready container exists it is assigned instantly (sub-100ms). Only on a pool miss does the request fall through to the existing scheduler queue.
- Pool size per node is capped at `WARM_POOL_SIZE` (total pre-warmed slots, not per-account).
- Idle containers that have not been assigned within `WARM_IDLE_TIMEOUT` are recycled (stopped and replaced) to prevent stale Chrome sessions and resource waste.
- Auto-replenishment: when a warm container is claimed, the pool immediately starts a replacement in the background.
- Profile isolation is maintained: each warm container is pre-started for a specific known `account_id` with that account's profile directory already mounted.
- New endpoint `GET /browser/pool/status` exposes pool depth, idle ages, and account assignments for ops visibility.

## Capabilities

### New Capabilities

- `warm-pool-lifecycle`: Maintain a set of pre-warmed, idle Docker Chrome containers mapped to specific account IDs; assign instantly on `POST /browser/start`; replenish asynchronously after assignment.
- `warm-pool-recycler`: Detect and evict idle containers that exceed `WARM_IDLE_TIMEOUT`; restart recycled slots as fresh warm containers; expose pool health via status endpoint.

### Modified Capabilities

- `browser-container-lifecycle`: `POST /browser/start` acquires from warm pool before submitting to the scheduler queue. Response is now immediate (HTTP 200 with container info) on a pool hit, and HTTP 202 (job reference) on a pool miss. Requires delta spec.

## Impact

- **Code**: New `internal/browser/warm_pool.go`; `internal/server/browser_handlers.go` updated to check pool before calling `Scheduler.Submit()`; new `GET /browser/pool/status` handler.
- **APIs**: `POST /browser/start` can now return HTTP 200 synchronously (pool hit) or HTTP 202 async (pool miss) — clients must handle both.
- **Config**: New env vars `WARM_POOL_SIZE` (default 3), `WARM_IDLE_TIMEOUT` (default 10m), `WARM_POOL_ACCOUNTS` (comma-separated account IDs to pre-warm; defaults to all active accounts).
- **Resources**: Each pre-warmed container consumes a port pair, a Chrome process, and Xvfb/VNC — `WARM_POOL_SIZE` must be set within the node's capacity (`MAX_CONCURRENT_BROWSERS` accounts for pool slots).
- **No new external dependencies** — uses existing Docker SDK and `PortRegistry`.

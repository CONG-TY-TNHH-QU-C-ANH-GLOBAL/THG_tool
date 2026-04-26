## ADDED Requirements

### Requirement: Facebook group/post/profile crawl execution
The system SHALL implement `FacebookCrawlHandler` that handles `job.Type = "facebook_crawl"`. It SHALL unmarshal `TaskJSON` from `job.Payload`, acquire a browser container via `BrowserRuntimeManager.StartContainer`, iterate `crawl_plan.sources`, and for each source execute the fetch loop with per-item filter and dedup evaluation.

#### Scenario: Group crawl collects filtered records
- **WHEN** `FacebookCrawlHandler.Handle` processes a task with one `facebook_group` source and `sampling.max_total_items=50`
- **THEN** the handler navigates to the group, batches posts, applies `FilterEngine.Evaluate` to each, accumulates up to 50 passing records, and writes `OutputDataset` to `store.CompleteTask`

#### Scenario: Sampling limit stops the fetch loop early
- **WHEN** `sampling.max_total_items=30` and the handler has accumulated 30 passing records before exhausting all sources
- **THEN** the handler stops the fetch loop immediately and does not navigate additional sources or fetch additional batches

### Requirement: In-crawl filter application (not post-collection)
`FilterEngine.Evaluate` SHALL be called on each item as it is fetched, before it is added to the `records` accumulator. No batch of raw items SHALL be collected and then filtered.

#### Scenario: Filter applied before accumulation
- **WHEN** a batch of 20 raw items is fetched from a Facebook group page
- **THEN** `FilterEngine.Evaluate` is called 20 times in the same loop iteration; items with `Pass=false` are discarded immediately; only passing items are appended to `records`

### Requirement: Inter-batch delay
The handler SHALL sleep `task.batching.inter_batch_delay_ms` between consecutive page-fetch iterations for the same source. This delay SHALL be applied after processing each batch and before fetching the next.

#### Scenario: Delay applied between batches
- **WHEN** `batching.inter_batch_delay_ms=2000` and the handler fetches batch 1 of a group
- **THEN** the handler sleeps 2000ms before fetching batch 2

### Requirement: Browser container acquired and released per task
The handler SHALL call `BrowserRuntimeManager.StartContainer(accountID, orgID)` at the start of execution and `BrowserRuntimeManager.StopContainer(accountID, orgID)` in a `defer` at the end (even on error). The handler SHALL wait up to `CRAWL_BROWSER_START_TIMEOUT` (default 30s) for `browser_containers.state = "running"`.

#### Scenario: Browser timeout fails task
- **WHEN** `BrowserRuntimeManager.StartContainer` does not produce `state="running"` within `CRAWL_BROWSER_START_TIMEOUT`
- **THEN** the handler returns an error; `store.FailTask` is called; the scheduler retry policy applies; the container (if partially started) is cleaned up by `BrowserRuntimeManager.StopContainer` in the deferred call

### Requirement: Handler does not parse intent or modify TaskJSON
The handler receives a fully-specified `TaskJSON` and executes it as given. It SHALL NOT re-interpret the task text, call `AITaskParser`, or modify any field of `TaskJSON`.

#### Scenario: Handler executes exactly what the task specifies
- **WHEN** `TaskJSON.crawl_plan.sources` contains two URLs
- **THEN** the handler visits exactly those two URLs in the specified order; it does not discover or add additional sources

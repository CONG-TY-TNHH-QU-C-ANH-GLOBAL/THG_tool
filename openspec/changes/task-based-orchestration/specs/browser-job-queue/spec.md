## REMOVED Requirements

### Requirement: `skill_run` job type
**Reason**: The skill-based execution model is removed. `skill_run` jobs, `SkillRunHandler`, and all skill idempotency key schemes are deleted. Execution is now driven by intent-typed crawl handlers.
**Migration**: Replace `jobs.Submit("skill_run", ...)` call sites with `JobGenerator.Submit(taskJSON)`. Register `FacebookCrawlHandler`, `LeadGenHandler`, `VisaResearchHandler`, `WebCrawlHandler` in place of `SkillRunHandler`.

## ADDED Requirements

### Requirement: Four crawl job types registered at startup
The system SHALL register the following job types in `jobs.Registry` at application startup, before `jobs.Scheduler.Start()`:

| job type | handler | max_attempts | backoff | retry_delay |
|---|---|---|---|---|
| `facebook_crawl` | `FacebookCrawlHandler` | 2 | constant | 60s |
| `lead_generation` | `LeadGenHandler` | 2 | constant | 60s |
| `visa_research` | `VisaResearchHandler` | 2 | constant | 60s |
| `web_crawl` | `WebCrawlHandler` | 2 | constant | 60s |

#### Scenario: Crawl job claimed and dispatched
- **WHEN** a `scheduler_jobs` row with `type="facebook_crawl"` and `status="pending"` exists
- **THEN** a worker goroutine claims it via the atomic subquery UPDATE and dispatches to `FacebookCrawlHandler.Handle(ctx, job)`

#### Scenario: Crawl job and browser_start job coexist in queue
- **WHEN** both `browser_start` and `facebook_crawl` pending jobs exist simultaneously
- **THEN** workers claim by `created_at ASC`; neither type has priority; both are processed by the same worker pool

### Requirement: Crawl job idempotency key is the task_id
The idempotency key for all crawl job types SHALL be `task.TaskID` (the deterministic content hash computed by `AITaskParser`). This guarantees that submitting the same task twice produces one `scheduler_jobs` row.

#### Scenario: Duplicate task submission idempotent
- **WHEN** `jobs.Submit("facebook_crawl", "a3f7c2e1b9d40851", payload)` is called twice
- **THEN** `INSERT OR IGNORE` no-ops on the second call; one `scheduler_jobs` row exists; one execution occurs

### Requirement: Crawl job retry does not re-run insight pipeline
On retry (handler returns error, attempt < max_attempts), the handler re-executes the full crawl from the beginning. Previously accumulated `leads` table rows written during the failed attempt are protected by dedup — they are not re-inserted on retry.

#### Scenario: Retry re-runs crawl, dedup prevents duplicate writes
- **WHEN** a `facebook_crawl` job fails after accumulating 10 records and is reset to `pending`
- **THEN** on retry, the handler re-fetches all sources; `DuplicateChecker.IsNew` returns `false` for the 10 already-written records; they are not re-inserted; new records from the remaining sources are added normally

## ADDED Requirements

### Requirement: Lead generation crawl with mandatory scoring
The system SHALL implement `LeadGenHandler` that handles `job.Type = "lead_generation"`. It SHALL execute the same Facebook crawl logic as `FacebookCrawlHandler` and additionally call `InsightPipeline.Run` with `lead_scoring` as a mandatory insight (regardless of `task.Output.Insights`).

#### Scenario: Lead scoring always runs for lead_generation tasks
- **WHEN** `LeadGenHandler.Handle` executes and `task.Output.Insights` is empty
- **THEN** `InsightPipeline.Run(records, ["lead_scoring"])` is called anyway; lead scores are included in `OutputDataset.insights.lead_scores`

#### Scenario: Additional requested insights combined with mandatory scoring
- **WHEN** `task.Output.Insights = ["summary"]`
- **THEN** `InsightPipeline.Run(records, ["lead_scoring", "summary"])` is called; both lead scores and summary are in the output

### Requirement: Qualified leads written to `leads` table
After `InsightPipeline.Run` returns `lead_scores`, the handler SHALL write records with `score >= LEAD_SCORE_THRESHOLD` (default 0.7) to the `leads` table with `status="pending_review"`, `org_id`, `account_id`, `source_task_id=task.TaskID`, `score`, `source_url`, and `author` fields.

#### Scenario: High-score leads written
- **WHEN** `InsightPipeline` returns scores `[{record_id:"A", score:0.87}, {record_id:"B", score:0.45}]` and `LEAD_SCORE_THRESHOLD=0.7`
- **THEN** only record A is written to `leads` with `status="pending_review"`; record B is included in `OutputDataset` with its score but not written to `leads`

#### Scenario: No qualified leads — no leads table writes
- **WHEN** all scored records have `score < LEAD_SCORE_THRESHOLD`
- **THEN** no rows are written to `leads`; the `OutputDataset` still contains `lead_scores` for all records

### Requirement: Lead deduplication against existing leads table
Before writing a new lead, the handler SHALL check `SELECT 1 FROM leads WHERE source_url=? AND org_id=?`. If a lead with the same `source_url` and `org_id` already exists, it SHALL skip writing and increment `stats.leads_deduped`.

#### Scenario: Duplicate lead skipped
- **WHEN** a record's `source_url` already exists in `leads` for the same `org_id`
- **THEN** no new row is inserted; `stats.leads_deduped++`

### Requirement: LeadGenHandler is read-only toward lead content
The handler SHALL NOT generate, post, or send any content. It classifies and stores lead candidates only. Comment generation, DM sending, and engagement are separate concerns outside this system.

#### Scenario: LeadGenHandler makes no write to posts or outbound_messages
- **WHEN** `LeadGenHandler.Handle` completes successfully
- **THEN** only `leads` rows (qualified records) and the `tasks.result_json` field are written; `posts` and `outbound_messages` tables are untouched

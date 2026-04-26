## ADDED Requirements

### Requirement: Structured dataset as sole output format
All task handlers SHALL write exactly one `OutputDataset` JSON object to `tasks.result_json`. The dataset SHALL contain `records` (filter-passing, deduplicated items only), `stats` (fetch and return counts), and `insights` (empty object if no insights requested).

#### Scenario: Output contains only passing records
- **WHEN** a handler fetches 200 items, 153 fail filters, 5 are duplicates, 42 pass
- **THEN** `OutputDataset.records` contains exactly 42 items; `stats.total_fetched=200`; `stats.total_returned=42`; the 158 rejected items appear nowhere in the output

#### Scenario: Empty result is valid
- **WHEN** a handler completes but all fetched items fail filters
- **THEN** `OutputDataset.records=[]`, `stats.total_returned=0`; the task is marked `completed` (not failed); the caller receives an empty dataset with stats

### Requirement: `stats.total_fetched` is internal — not user-visible in API response
`stats.total_fetched` SHALL be stored in `tasks.result_json` for internal audit purposes but SHALL be omitted from the `GET /api/v1/tasks/:id` API response. The API response includes only `total_returned`. This prevents exposing how many items were filtered out (potential signal of data collection volume).

#### Scenario: API response omits total_fetched
- **WHEN** `GET /api/v1/tasks/42` is called
- **THEN** the `stats` object in the response contains `total_returned` and optionally `dedup_count` and `leads_written`; `total_fetched` is not present

### Requirement: Record fields are intent-specific but share a common base
All records SHALL include base fields: `id` (dedup_hash), `source_url`, `source_type`, `timestamp`, `author.name`, `author.profile_url`. Intent-specific fields are additive:
- `facebook_crawl` / `lead_generation`: adds `content`, `engagement{reactions,comments,shares}`, `filter_signals`
- `visa_research`: adds `title`, `deadline_date`, `fee_amount`, `document_list`, `office_address`, `contact_info`
- `web_crawl`: adds `title`, `description`, `price_signals`, `contact_info`

#### Scenario: Facebook record has content and engagement
- **WHEN** `OutputDataset.records` is written by `FacebookCrawlHandler`
- **THEN** each record contains `content`, `engagement.reactions`, `engagement.comments`, and `filter_signals`

#### Scenario: Visa record has structured service fields
- **WHEN** `OutputDataset.records` is written by `VisaResearchHandler`
- **THEN** each record contains `title` and at least one of `fee_amount`, `deadline_date`, `document_list`

### Requirement: `insights` object is empty when not requested
When `task.Output.Insights` is empty and the handler does not mandate insights (only `LeadGenHandler` mandates `lead_scoring`), `OutputDataset.insights` SHALL be `{}` — not `null` and not omitted.

#### Scenario: No insights requested returns empty object
- **WHEN** `task.Output.Insights=[]` and the handler is `FacebookCrawlHandler`
- **THEN** `OutputDataset.insights={}` in `tasks.result_json`

### Requirement: No raw scraping output in any form
No handler SHALL write raw HTML, raw API response JSON, unfiltered item arrays, or intermediate parsing state to `tasks.result_json` or any other table.

#### Scenario: Raw HTML never stored
- **WHEN** a handler fetches a Facebook group page
- **THEN** only structured `Record` structs derived from the page are written; the raw HTML string is never persisted

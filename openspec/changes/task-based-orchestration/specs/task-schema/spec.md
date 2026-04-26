## ADDED Requirements

### Requirement: Versioned task document
Every task processed by the system SHALL conform to TaskSchema version `"1"`. The `schema_version` field SHALL be the first field validated. Any document with an unknown version SHALL be rejected by `JobGenerator` before any DB write.

#### Scenario: Valid v1 document accepted
- **WHEN** `JobGenerator.Submit` receives a `TaskJSON` with `schema_version="1"` and all required fields populated
- **THEN** validation passes and submission proceeds

#### Scenario: Unknown version rejected
- **WHEN** `JobGenerator.Submit` receives a `TaskJSON` with `schema_version="2"`
- **THEN** an `ErrSchemaValidation{Field:"schema_version", Reason:"unknown version"}` is returned; no DB write occurs

### Requirement: Intent field maps to a registered handler
`task.intent` SHALL be one of the registered task types in `TaskRegistry` at submission time. The set of valid intents at launch: `facebook_crawl`, `lead_generation`, `visa_research`, `web_crawl`.

#### Scenario: Valid intent accepted
- **WHEN** `TaskJSON.intent = "lead_generation"` and `LeadGenHandler` is registered
- **THEN** validation passes

#### Scenario: Unknown intent rejected
- **WHEN** `TaskJSON.intent = "unknown_task"` and no handler is registered for it
- **THEN** `ErrSchemaValidation{Field:"intent"}` returned; no `tasks` row or `scheduler_jobs` row created

### Requirement: Source URL safety validation
Every URL in `crawl_plan.sources` SHALL be validated: must use `https://` scheme; must not resolve to `localhost`, `127.0.0.1`, `0.0.0.0`, or any RFC-1918 address (`10.x`, `172.16–31.x`, `192.168.x`); must not be a `file://` or `data://` URI.

#### Scenario: HTTPS public URL accepted
- **WHEN** `crawl_plan.sources[0].url = "https://www.facebook.com/groups/shiphangmy"`
- **THEN** URL passes validation

#### Scenario: Private IP URL rejected
- **WHEN** `crawl_plan.sources[0].url = "https://192.168.1.1/admin"`
- **THEN** `ErrSchemaValidation{Field:"crawl_plan.sources[0].url", Reason:"private IP not allowed"}` returned

### Requirement: Sampling hard cap enforcement
`crawl_plan.sampling.max_total_items` SHALL NOT exceed `CRAWL_MAX_ITEMS_HARD_LIMIT` (default 1000). Values above the cap are rejected at validation time, not silently clamped.

#### Scenario: Over-limit sampling rejected
- **WHEN** `sampling.max_total_items = 5000` and `CRAWL_MAX_ITEMS_HARD_LIMIT = 1000`
- **THEN** `ErrSchemaValidation{Field:"crawl_plan.sampling.max_total_items", Reason:"exceeds hard limit 1000"}` returned

### Requirement: Source type constraint per intent
`crawl_plan.sources[].type` SHALL be compatible with the declared `intent`. `facebook_crawl` and `lead_generation` MUST use only `facebook_group`, `facebook_post`, or `facebook_profile` types. `visa_research` and `web_crawl` MUST use only `web_url` type.

#### Scenario: Intent-source mismatch rejected
- **WHEN** `intent="visa_research"` and `crawl_plan.sources[0].type="facebook_group"`
- **THEN** `ErrSchemaValidation{Field:"crawl_plan.sources[0].type"}` returned; the visa handler does not know how to crawl Facebook groups

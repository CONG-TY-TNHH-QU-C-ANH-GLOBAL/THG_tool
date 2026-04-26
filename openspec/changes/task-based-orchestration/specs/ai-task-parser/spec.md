## ADDED Requirements

### Requirement: Structured task extraction from free text
The system SHALL implement `AITaskParser.Parse(ctx context.Context, text string, orgID int64, accountID int64) (*TaskJSON, ParseTokenUsage, error)` that calls GPT-4o in structured output mode with `TaskSchema v1` as the `response_format` JSON Schema. The parser SHALL extract `intent`, `crawl_plan.sources`, `crawl_plan.query`, `crawl_plan.time_range`, `crawl_plan.sampling`, `filters`, `batching`, and `output.insights` from the free-text input.

#### Scenario: Unambiguous Vietnamese crawl command
- **WHEN** `AITaskParser.Parse(ctx, "cào nhóm ship hàng mỹ trong tháng này https://fb.com/groups/abc", 7, 42)` is called
- **THEN** the parser returns a `TaskJSON` with `intent="facebook_crawl"`, `crawl_plan.sources=[{type:"facebook_group", url:"https://fb.com/groups/abc"}]`, `crawl_plan.query.keywords` containing ["ship hàng mỹ"], and `crawl_plan.time_range` scoped to the current month

#### Scenario: POD customer discovery intent
- **WHEN** `AITaskParser.Parse(ctx, "tìm khách hàng POD tiềm năng trên Facebook quan tâm áo thun in", ...)` is called
- **THEN** the parser returns `intent="lead_generation"`, keywords containing ["POD", "áo thun in"], and filters with language `["vi"]`

#### Scenario: Visa research intent
- **WHEN** `AITaskParser.Parse(ctx, "tìm thông tin visa Nhật Bản 2026 phí và hồ sơ", ...)` is called
- **THEN** the parser returns `intent="visa_research"`, `crawl_plan.sources` with `type="web_url"` entries for visa-related URLs extracted from the query, and `output.insights` containing `["summary"]`

### Requirement: Server-side injection of identity fields
The system SHALL inject `task_json.created_by.org_id` and `task_json.created_by.account_id` from the authenticated server context, NOT from the LLM output. The LLM response for `created_by` SHALL be ignored and overwritten.

#### Scenario: org_id always comes from server context
- **WHEN** the LLM response contains `created_by.org_id=99` but the authenticated caller has `org_id=7`
- **THEN** the returned `TaskJSON.created_by.org_id` is `7`; the LLM value is discarded

### Requirement: Deterministic task_id computation
The parser SHALL compute `task_id = hex(sha256(string(accountID) + intent + join(sorted(keywords)) + date_utc_day()))[:16]`. The same account, same intent, same keywords on the same day MUST produce the same `task_id`.

#### Scenario: Identical command produces same task_id
- **WHEN** the same user sends the same crawl command twice in the same day
- **THEN** both calls to `AITaskParser.Parse` return `TaskJSON` with identical `task_id` values

#### Scenario: Different day produces different task_id
- **WHEN** the same command is sent on 2026-04-25 and 2026-04-26
- **THEN** the two `TaskJSON` values have different `task_id` values

### Requirement: Typed error returns
The parser SHALL return `ErrAmbiguous{Clarification string}` when the LLM cannot extract a complete task (no clear intent or sources). It SHALL return `ErrUnsupportedIntent{Intent string}` when the model returns an intent string not registered in `TaskRegistry`.

#### Scenario: Ambiguous input returns clarification
- **WHEN** `AITaskParser.Parse(ctx, "làm gì đó trên facebook", ...)` is called and the model cannot determine intent or sources
- **THEN** the parser returns `ErrAmbiguous{Clarification: "Bạn muốn cào nhóm nào? Vui lòng cung cấp link nhóm hoặc từ khóa cụ thể."}` and `nil` for `TaskJSON`

#### Scenario: Token usage always returned
- **WHEN** `AITaskParser.Parse` returns any result (success or error)
- **THEN** `ParseTokenUsage{PromptTokens int, CompletionTokens int}` is populated from `response.Usage`; it is never zero on a successful API call

### Requirement: Stateless, no side effects
The parser SHALL NOT write to any database table, call any handler, submit any job, or touch the file system. It is a pure transformation function.

#### Scenario: Parser does not write to DB
- **WHEN** `AITaskParser.Parse` is called successfully
- **THEN** no rows are inserted or updated in any SQLite table; the only external call is to the OpenAI API

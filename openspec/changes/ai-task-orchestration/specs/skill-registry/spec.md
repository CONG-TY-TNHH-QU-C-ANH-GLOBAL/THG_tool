## ADDED Requirements

### Requirement: Skill interface and registration
The system SHALL define a `Skill` interface with methods `Name() string`, `Description() string`, `ParamSchema() map[string]any`, and `Run(ctx context.Context, accountID int64, params map[string]any) (SkillResult, error)`. `SkillRegistry.Register(skill Skill) error` SHALL store the skill under its `Name()`. Registering the same name twice SHALL return an error.

#### Scenario: Skill registered and retrievable
- **WHEN** `registry.Register(scrapeGroupSkill)` is called at startup
- **THEN** `registry.Get("scrape_group")` returns the skill and `registry.All()` includes it in the list

#### Scenario: Duplicate registration rejected
- **WHEN** `registry.Register` is called twice with skills that return the same `Name()`
- **THEN** the second call returns a non-nil error; the first registration is unchanged

### Requirement: Parameter schema exposure for routing
Each registered skill SHALL declare its expected parameters as a JSON Schema-compatible `map[string]any` via `ParamSchema()`. The router uses this to build OpenAI function definitions. The schema SHALL include `"type": "object"`, `"properties"`, and `"required"` fields.

#### Scenario: Schema used in function definition
- **WHEN** `SkillRegistry.All()` is called and the result is passed to `SkillRouter`
- **THEN** each skill's `ParamSchema()` is embedded as the `parameters` field of the corresponding OpenAI function definition

#### Scenario: Missing required param caught before execution
- **WHEN** a routed `params` map is missing a field listed in the skill's `"required"` array
- **THEN** `TaskExecutor` returns a validation error before calling `skill.Run`; no `tasks` row is written and no browser work begins

### Requirement: SkillResult structure
`SkillResult` SHALL carry `Summary string` (human-readable outcome for Telegram reply) and `Data any` (structured result, JSON-serialized into `tasks.result_json`). Skills that perform actions (comment, send DM) set `Data` to nil. Skills that produce records (scrape, profile fetch) set `Data` to a typed slice or struct.

#### Scenario: Action skill returns summary only
- **WHEN** `post_comment.Run` completes successfully
- **THEN** `SkillResult.Summary = "Đã đăng bình luận trên bài viết"` and `SkillResult.Data = nil`

#### Scenario: Scrape skill returns structured data
- **WHEN** `scrape_group.Run` completes and finds 12 posts
- **THEN** `SkillResult.Summary = "Đã cào 12 bài viết từ nhóm"` and `SkillResult.Data` is a `[]Post` slice

### Requirement: Thread-safe registry
`SkillRegistry` SHALL be safe for concurrent reads after all registrations complete at startup. `Register` calls after the scheduler starts are NOT required to be safe; all registrations happen before `main` calls `scheduler.Start()`.

#### Scenario: Concurrent reads after startup
- **WHEN** multiple worker goroutines simultaneously call `registry.Get(skillName)` during task execution
- **THEN** all calls return consistent results with no data race (verified by `-race` flag in tests)

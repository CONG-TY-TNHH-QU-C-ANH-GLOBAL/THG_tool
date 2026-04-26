## ADDED Requirements

### Requirement: Natural language to skill routing
The system SHALL accept a free-text string in any language (primarily Vietnamese) and return a resolved `{skillName string, params map[string]any}` by calling the OpenAI Chat Completions API with all registered skill schemas as function definitions. The system SHALL use the `OPENAI_MODEL` environment variable to select the model.

#### Scenario: Unambiguous command resolved
- **WHEN** `SkillRouter.Route(ctx, "cào group ship hàng mỹ https://facebook.com/groups/abc")` is called
- **THEN** the router returns `skillName="scrape_group"`, `params={"url":"https://facebook.com/groups/abc"}`, `err=nil`

#### Scenario: Command with implicit account context
- **WHEN** `SkillRouter.Route(ctx, "post bình luận vào bài viết này: https://fb.com/post/123")` is called
- **THEN** the router returns `skillName="post_comment"`, `params={"url":"https://fb.com/post/123"}` (account_id resolved by TaskExecutor, not the router)

### Requirement: Clarification on ambiguous or incomplete input
The system SHALL detect when the model cannot confidently route the input (no tool_call returned, finish_reason="stop") and return an `ErrNeedsMoreContext` error carrying the model's natural language response as the clarification message.

#### Scenario: Ambiguous input triggers clarification
- **WHEN** `SkillRouter.Route(ctx, "làm việc trên facebook")` is called and the model returns a text reply instead of a tool call
- **THEN** `Route` returns `ErrNeedsMoreContext{Message: <model reply>}`; the caller presents the message to the user and awaits a follow-up

#### Scenario: Completely unrelated input rejected
- **WHEN** `SkillRouter.Route(ctx, "hôm nay thời tiết thế nào")` is called
- **THEN** the router returns `ErrUnroutableIntent`; no skill is dispatched; no API key cost beyond the single classification call

### Requirement: Dynamic schema registration
The system SHALL build the OpenAI function definitions list by calling `SkillRegistry.All()` at each `Route` invocation so that newly registered skills are automatically routable without modifying the router.

#### Scenario: Newly registered skill is immediately routable
- **WHEN** a new skill is registered via `SkillRegistry.Register` after `SkillRouter` is constructed
- **THEN** the next call to `SkillRouter.Route` includes the new skill's `ParamSchema()` in the function definitions sent to the model

### Requirement: Token usage tracking
The system SHALL return the prompt and completion token counts from the OpenAI response alongside the routing result so the caller can record them in the `tasks` table.

#### Scenario: Token counts returned on successful route
- **WHEN** `SkillRouter.Route` returns `(skillName, params, nil)`
- **THEN** the return value also includes `promptTokens int` and `completionTokens int` reflecting the usage from `response.Usage`

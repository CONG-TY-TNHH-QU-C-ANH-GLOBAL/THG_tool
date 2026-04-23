---
description: How the Telegram AI Agent works — prompt processing, function calling, action routing
---

# Telegram AI Agent

## Architecture
```
User Message → Telegram Bot → AI Agent (gpt-4.1) → Function Call → Orchestrator.HandleAgentAction()
                                                                          ↓
                                                                    Execute Action → Notify via Telegram
```

## File: `internal/ai/agent.go`

### Agent.ProcessPrompt(ctx, prompt, source)
1. Load user context from DB (business rules, constraints)
2. Build dynamic system prompt with rules + few-shot examples
3. Send to OpenAI with function definitions (`agentTools`)
4. Parse response: function_call → extract name + arguments
5. Call `ActionHandler(action, args)` → Orchestrator executes
6. Return AI response text to Telegram

### System Prompt Structure
The system prompt (800+ lines) contains:
- Business rules and identity
- 27+ specific trigger-to-function mappings
- Anti-spam instructions
- Position extraction rules for `post_jds_to_groups`

### Available Functions (agentTools)
| Function | Description | Key Parameters |
|---|---|---|
| `scrape_group` | Scrape posts from a Facebook group | `group_url` |
| `scrape_comments` | Scrape comments from a post | `post_url` |
| `check_inbox` | Check Messenger inbox | - |
| `get_stats` | System statistics | - |
| `classify_leads` | AI classify scraped posts | - |
| `send_comment` | Comment on a post | `post_url`, `content` |
| `create_job_post` | Create a single JD post | `title`, `description`, ... |
| `full_recruitment_pipeline` | Full recruitment cycle | - |
| `post_jds_to_groups` | Post JDs to Facebook groups | `positions` (optional) |
| `scan_own_jd_posts` | Scan comments on posted JDs | - |
| `list_career_jobs` | List jobs in DB | - |
| `scrape_careers_page` | Scrape careers website | `url` |

### Action Routing: `orchestrator.HandleAgentAction(action, args)`
This massive switch statement (1000+ lines) in `orchestrator.go` handles every function call:
- Parses args from the AI
- Calls the appropriate subsystem
- Returns a response string to show on Telegram
- Runs long operations in goroutines with `context.WithTimeout`
- Sends progress notifications via `safeNotify()`

## Memory & Learning
- `learnFromPrompt()`: Extracts keywords from user prompts → stores as business context
- `getFewShotExamples()`: Retrieves similar past prompt→action pairs for in-context learning
- `logPrompt()`: Logs every prompt + response for debugging
- `updateMemory()`: Updates the few-shot memory bank

## File: `internal/telegram/bot.go`
- Receives messages from Telegram channel
- Routes to `Agent.ProcessPrompt()`
- Sends response back to channel
- Also sends proactive notifications from Orchestrator

## Adding New Agent Functions
1. Add function schema to `agentTools` array in `agent.go` (line ~480+)
2. Add trigger rules to `buildDynamicSystemPrompt()` (line ~213+)
3. Add handler case to `HandleAgentAction()` in `orchestrator.go` (line ~476+)
4. Build and test

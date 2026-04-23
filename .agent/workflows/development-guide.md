---
description: How to build, run, deploy, and debug this project
---

# Development Guide

## Prerequisites
- Go 1.21+
- Google Chrome installed
- OpenAI API key
- Telegram bot token + channel ID

## Build & Run
// turbo-all
```bash
# Build
go build -o scraper.exe ./cmd/scraper

# Run (uses .env for config)
.\scraper.exe

# Or combined
go build -o scraper.exe ./cmd/scraper; .\scraper.exe
```

## Build Check (Lint)
```bash
go build ./... 2>&1
```

## Database
- **File**: `data/scraper.db` (SQLite)
- **Migrations**: Auto-run in `store.New()` in `internal/store/store.go`
- **Schema changes**: Add migration SQL in `store.go`'s `migrate()` function

## Key Files to Edit for Common Tasks

### Add a new Telegram command
1. `internal/ai/agent.go` — `buildDynamicSystemPrompt()`: Add trigger rule
2. `internal/ai/agent.go` — `agentTools`: Add function schema
3. `internal/orchestrator/orchestrator.go` — `HandleAgentAction()`: Add handler

### Fix Facebook posting/commenting
1. `internal/scraper/autocomment.go` — The central file
2. Key methods: `PostToGroup()`, `PostCommentWithImage()`, `PostComment()`
3. Debug: Check logs for `[AutoComment]` prefix

### Fix lead classification
1. `internal/ai/classifier.go` — Classification prompt + model
2. `internal/orchestrator/orchestrator.go` — `handleScrapePostsJob()` calls classifier

### Fix group domain matching
1. `internal/orchestrator/recruitment_pipeline.go` — `normalizeGroupCategory()`
2. `internal/ai/group_scorer.go` — `ScoreGroupQuality()` returns categories

### Change AI model
1. `internal/config/config.go` — Default model values
2. `cmd/scraper/main.go` — Which component gets which model
3. Current mapping:
   - Agent + MessageGenerator → `OPENAI_COMMENT_MODEL` (gpt-4.1)
   - Classifier + Pricer → `OPENAI_MODEL` (gpt-4o-mini)

### Add a new database table
1. `internal/models/` — Add model struct
2. `internal/store/store.go` — Add migration + CRUD methods

### Modify Telegram notifications
1. `internal/orchestrator/orchestrator.go` — `safeNotify()` calls
2. Search for emoji patterns like `✅`, `❌`, `📝` to find notification points

## Debugging Tips

### Log Prefixes
| Prefix | Component |
|---|---|
| `[AutoComment]` | Facebook posting/commenting |
| `[Orchestrator]` | Job processing, routing |
| `[PostJDs]` | JD posting pipeline |
| `[AI Classifier]` | Lead classification |
| `[Agent]` | Telegram AI agent |
| `[Telegram]` | Bot message handling |
| `[Queue]` | Job queue processing |

### Common Debug Workflow
1. User triggers via Telegram → check `[Telegram]` + `[Agent]` logs
2. Agent calls function → check `[Orchestrator]` logs
3. Job queued → check `[Queue]` + `[Worker-N]` logs
4. Facebook interaction → check `[AutoComment]` logs
5. If posting fails → look for `Compose info:`, `Dialog check:`, `Submit info:` logs

## Deployment
- **Docker**: `Dockerfile` in root
- **CI/CD**: `.github/workflows/` — GitHub Actions → SSH deploy to VPS
- **Process Manager**: PM2 on VPS
- **Playwright deps**: Install on headless server for Chrome

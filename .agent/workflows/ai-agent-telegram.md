---
description: How the Telegram/Dashboard AI Agent routes open crawler prompts
---

# Telegram And Dashboard AI Agent

## Architecture

```
User message
  -> Telegram bot or dashboard chat
  -> AI Agent
  -> ActionHandler in cmd/scraper
  -> scheduler job
  -> worker open crawler
  -> classifier
  -> response + stored outputs
```

## Production Rules

- The agent is the only free-text path for crawler prompts.
- If `OPENAI_API_KEY` is missing, production free-text crawling is disabled instead of using keyword fallbacks.
- Broad requests like "scan all" must ask for a target URL/search/context first.
- `scrape_group` is a compatibility action that maps a concrete URL into a `facebook_crawl` job.
- `scrape_all` is intentionally unavailable as a production tool.
- Business context updates go through `describe_business` or `set_context`.

## Current Tool Surface

| Function | Production behavior |
|---|---|
| `scrape_group` | Submit a prompt-scoped `facebook_crawl` job for a concrete Facebook URL |
| `scrape_comments` | Submit a `facebook_crawl` job for comments on a concrete post URL |
| `describe_business` | Save business context used by classifier |
| `set_context` | Save operational flags/context |
| `get_stats` | Return dashboard/job/lead stats |
| outreach tools | Act only on explicit targets or already-classified leads |

## Action Handler

The active handler lives in `cmd/scraper/main.go` as `makeAgentActionHandler`.
It should stay thin: parse tool args, create durable open crawler jobs, and return
clear feedback. Long work belongs in `cmd/worker`.

## Memory And Learning

- Prompt history is useful for debugging and few-shot examples.
- Memories must not reintroduce fixed scan-all behavior.
- If a memory suggests an old action, the production override wins.

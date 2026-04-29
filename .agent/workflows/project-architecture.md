---
description: THG open crawler production architecture and module map
---

# THG Open Crawler - Production Architecture

## Mindset

THG is no longer a fixed "scan configured groups" scraper. Production work enters
through open prompts from Telegram or the dashboard chat. The AI agent parses the
operator's request, turns it into a durable crawler job, runs it inside the
selected Facebook workspace, and stores only classified, relevant outputs.

The important contract is:

1. User prompt defines the target, intent, filters, and expected output.
2. Browser automation reuses the real logged-in Facebook workspace for the chosen account.
3. Raw crawled items pass through business-aware classification before becoming leads.
4. No mock data, hidden local Chrome pool, or hardcoded fallback scan should decide production behavior.

## Runtime Flow

```
Telegram / Dashboard Chat
        |
        v
AI Agent Router
        |
        v
Open Crawler Job (scheduler_jobs)
        |
        v
Worker Handler (facebook_crawl / web_crawl / lead_gen)
        |
        v
Runtime Factory -> Workspace Chrome CDP session
        |
        v
Facebook / Web target visible in Browser tab
        |
        v
Universal Classifier -> Leads / Outputs / Audit logs
```

## Production Services

| Service | Binary | Responsibility |
|---|---|---|
| Backend API | `cmd/scraper` | Auth, REST API, WebSocket screen proxy, Telegram bot, job submission, workspace orchestration |
| Worker | `cmd/worker` | Durable job execution and open crawler handlers |
| Frontend | `frontend` (Next.js) | Dashboard UI, Browser page, chat/control surfaces |

Legacy `cmd/api`, embedded `internal/server/static`, hidden `browser.NewPool`, and
`internal/accounts` fallback account manager are intentionally removed. The backend
API and WebSocket surface is served by `cmd/scraper` on port `8080`.

## Key Modules

| Package | Purpose |
|---|---|
| `internal/server` | Gofiber API, auth/session endpoints, workspace handlers, screen WebSocket |
| `internal/workspace` | Starts and tracks persistent per-account Chrome profiles |
| `internal/runtime` | CDP runtime adapters used by crawler handlers |
| `internal/jobs` | Durable SQLite scheduler, idempotent submit/claim/retry |
| `internal/handlers/facebook_crawl` | Prompt-driven Facebook crawler handler |
| `internal/ai` | Agent router, message generation, universal lead classification |
| `internal/store` | SQLite persistence and migrations |
| `internal/telegram` | Telegram transport into the same AI/action pipeline |
| `frontend/src/modules/autoflow` | Production dashboard UI and browser observability |

## Crawler Contract

Crawler jobs should use `Intent: "facebook_crawl"` (or another open task intent),
not a fixed `scrape_group` skill. A task payload carries:

- `Source.Type`: `facebook_group`, `facebook_post`, `facebook_search`, or `web_url`
- `Source.URL` or query text supplied by the user
- optional `Keywords` derived from the prompt
- `Limit` from the prompt or API request
- `OutputSchema: "open_crawler_v1"`

`scrape_group` may exist only as a temporary compatibility alias for draining old
jobs, gated by `ENABLE_LEGACY_SCRAPE_GROUP=true` in the worker.

## Classification Contract

Every candidate item is first parsed into structured output, then classified
against the current business context. If OpenAI is configured, the universal
classifier can reject mismatched items before they become leads. If the model call
fails, deterministic scoring is allowed only as a fallback signal, not as the
primary product promise.

Context is maintained through agent actions:

- `set_context`
- `describe_business`
- future prompt-defined campaign settings

The classifier must prefer "reject" over false positives when intent, product,
region, or customer profile is unclear.

## Browser Visibility

Users must be able to observe Facebook automation from the dashboard Browser tab:

- one persistent Chrome profile per Facebook account
- real Facebook login saved in `data/profiles/account_<id>/`
- screen streamed through CDP WebSocket
- mouse, keyboard, and wheel events forwarded back to Chrome
- session validation prevents mixing the same account record with the wrong Facebook `c_user`

Production automation should attach to that visible workspace session instead of
launching a separate hidden Chrome.

## Anti-Patterns

- Reintroducing `cmd/api` or a second backend API on port `8081`
- Serving the old embedded `internal/server/static/index.html`
- Returning mock frontend data when production APIs fail
- Starting hidden local browser pools for real Facebook work
- Running `/scan_all` or configured-group loops without a user prompt target
- Hardcoding one scrape source as the main product path
- Creating leads before business-aware classification


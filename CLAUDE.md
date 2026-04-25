# THG Agentic Scraper — CLAUDE.md

## Project Overview

**Module**: `github.com/thg/scraper`  
**Language**: Go 1.26 + vanilla JS frontend  
**Binary**: `cmd/scraper/main.go`  
**Database**: SQLite (`data/scraper.db`) via `modernc.org/sqlite`  
**Web framework**: Gofiber v2  
**Browser automation**: chromedp  
**Telegram bot**: `gopkg.in/telebot.v3`  

This is a Facebook sales automation system. Staff control it via Telegram or a web dashboard. It scrapes Facebook groups for leads, classifies them with GPT-4o, then auto-comments on hot leads.

---

## Architecture

```
Telegram / Web Chat UI
        ↓
   AI Skill Router (natural language → skill name + params)
        ↓
  Skill Executor (Go function, runs in account's workspace Chrome)
        ↓
  WorkspaceManager (one Chrome process per FB account, persistent profile)
        ↓
  Facebook (logged in forever via Chrome user-data-dir)
```

### Internal Packages

| Package | Purpose |
|---|---|
| `internal/workspace` | **Workspace Chrome per account** — start/stop/attach Chrome per FB account |
| `internal/accounts` | Account manager — profile dirs, health checks, multi-account rotation |
| `internal/scraper` | chromedp-based Facebook scrapers (posts, profiles, comments) |
| `internal/ai` | OpenAI classifiers, message generator, selector healer, skill router |
| `internal/orchestrator` | Orchestrates scan cycles, processes agent results |
| `internal/queue` | Job queue — routes jobs to server Chrome or local agent |
| `internal/server` | Gofiber REST API + WebSocket + static file server |
| `internal/store` | SQLite CRUD — all DB access goes through here |
| `internal/telegram` | Telegram bot — receives commands, routes to skills |
| `internal/browser` | Chrome pool, VNC display manager (Linux only) |
| `internal/auth` | JWT access + refresh token auth |
| `internal/config` | .env → Config struct |
| `internal/models` | Shared data types |

### Key Files

- `cmd/scraper/main.go` — wires all packages together, starts goroutines
- `internal/server/api.go` — all HTTP routes, Server struct
- `internal/server/static/index.html` — SPA frontend (vanilla JS)
- `internal/server/static/app.js` — all frontend logic
- `internal/server/static/style.css` — dark theme CSS
- `internal/store/store.go` — main DB file (also imports `agent_tokens.go`, `selector_cache.go`)
- `internal/server/agent_handlers.go` — Chrome Extension agent API
- `internal/server/ws_hub.go` — WebSocket hub for Chrome Extension agents
- `internal/server/cdp_view.go` — CDP screencast (per-account, refactored)
- `internal/server/workspace_handlers.go` — workspace start/stop/view API

---

## Commercial Product Vision (autonow.vn style)

Reference product: https://autonow.vn — "AutoFB, AI Agent Quản Lý Facebook"

### Key Insight from autonow.vn

Each workspace = **one persistent Chrome profile per account** on the server.  
User logs into Facebook **once** via the live browser view (noVNC/CDP).  
Chrome saves the session in `data/profiles/account_{id}/`.  
All subsequent automation (scrape, comment, inbox) reuses that session — **no cookie import needed**.

### Autonow.vn UI Structure (replicate this)

```
Sidebar:
  Overview | Chat
  FACEBOOK:
    Browser   ← live Chrome view per account, login here
    Groups
    Leads
    Sentiment
  CONFIG:
    Coordinator
    Settings
    Logs
```

**Browser page** (most important):
- Shows all Facebook accounts with status pill (running/offline)
- Click account → opens CDP live view canvas
- ALREADY RUNNING / START / STOP / Refresh buttons
- "cdp: PORT · vnc: PORT" status line
- Canvas displaying JPEG frames from CDP screencast WebSocket
- Mouse + keyboard forwarded back through WebSocket → CDP input dispatch

---

## Phase 1: Workspace Chrome Per Account ✅ (BUILD FIRST)

**Status**: In progress / partially implemented

### What was built

1. `internal/workspace/workspace.go` — `Manager` struct that:
   - Keeps map of `accountID → *Instance` (running Chrome processes)
   - `Start(accountID, name)` → launches Chrome with free CDP port, `data/profiles/account_{id}/` profile
   - `Stop(accountID)` → kills Chrome
   - `Get(accountID)` → returns running instance (nil if not running)
   - `List()` → all running instances

2. `internal/server/cdp_view.go` — refactored to per-account hubs:
   - `cdpHubs map[int64]*cdpViewHub` in Server struct
   - `getAccountHub(id)` / `getOrCreateAccountHub(id)`
   - `startAccountScreencast(id, cdpPort)` → connects to Chrome CDP, starts JPEG streaming
   - `stopAccountScreencast(id)` → disconnects

3. `internal/server/workspace_handlers.go`:
   - `GET /api/browser/workspaces` → list accounts + running status
   - `POST /api/browser/workspaces/:id/start` → start Chrome for account
   - `POST /api/browser/workspaces/:id/stop` → stop Chrome
   - `POST /api/browser/workspaces/:id/navigate` → navigate to URL
   - `GET /ws/browser-view/:id` → WebSocket for live JPEG frames + input relay

4. Browser page in `index.html` — professional UI matching autonow.vn:
   - Account list with status pills
   - Live canvas view with mouse/keyboard forwarding
   - Start/Stop/Refresh controls

### Wiring in main.go

```go
workspaceMgr := workspace.NewManager(cfg.ChromePath, cfg.ProfileDir)
srv := server.New(db, q, agent, workspaceMgr, server.Config{...})
defer workspaceMgr.StopAll()
```

---

## Phase 2: Skills System (NEXT)

Replace the raw job queue with a modular skills system.

### Skill interface

```go
type Skill interface {
    Name() string
    Run(ctx context.Context, accountID int64, params map[string]any) (SkillResult, error)
}
```

### Skills to implement

| Skill | What it does |
|---|---|
| `scrape_group` | Scrape posts from a Facebook group URL |
| `post_comment` | Post a comment on a Facebook post URL |
| `send_inbox` | Send a DM to a Facebook user |
| `check_notifications` | Check Facebook notifications |
| `get_profile_info` | Get public info from a Facebook profile URL |
| `comment_hot_leads` | Comment on all hot leads not yet commented |

### Skill Router (AI)

```go
type SkillRouter struct {
    client *openai.Client
}
// Routes natural language → skill name + params
func (r *SkillRouter) Route(ctx context.Context, text string) (skillName string, params map[string]any, err error)
```

Prompt → GPT-4o (function calling) → `{"skill": "scrape_group", "params": {"url": "..."}}` → execute.

### Telegram integration

```
User: "cào group ship hàng mỹ"
Bot → SkillRouter.Route() → scrape_group(url=...)
Bot → WorkspaceMgr.Get(defaultAccountID)
Bot → Skill.Run(ctx, accountID, params)
Bot → "✅ Đã cào 47 bài, tìm được 12 leads"
```

---

## Phase 3: Commercial Features (LATER)

- Multi-workspace (each user/team isolated — own accounts, own Chrome)
- Sentiment analysis page (classify post sentiment, not just leads)
- Logs page (real-time skill execution log stream)
- Skill call history with cost metering
- Schedule skills (cron)
- API for developers (POST /api/skills/run)

---

## Data Models

Key tables in SQLite:

```sql
accounts         -- Facebook accounts (id, name, status, platform, cookies_json_encrypted)
groups           -- Facebook groups to monitor (url, platform, active)
posts            -- Scraped posts (content, author, group, dedup_hash)
leads            -- AI-classified leads (score: hot/warm/cold, source_url, author)
outbound_messages -- Auto-comments + DMs queue (status: draft/approved/sent/failed)
jobs             -- Job queue (type, target, status, execution_mode: server/local)
agent_tokens     -- Chrome Extension agent auth tokens (hashed)
selector_cache   -- CSS selectors per action+platform (self-healing via GPT-4o Vision)
users            -- Staff accounts (email, password_hash, role: admin/sales)
niches           -- Lead classification niches (slug, name, emoji)
```

---

## Environment Variables (.env)

```env
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o
OPENAI_COMMENT_MODEL=gpt-4o
TELEGRAM_BOT_TOKEN=...
TELEGRAM_ADMIN_CHAT=...
JWT_SECRET=...
ENCRYPTION_KEY=...
WEB_PORT=8080
DB_PATH=data/scraper.db
CHROME_PATH=/usr/bin/google-chrome
PROFILE_DIR=data/profiles
MAX_WORKERS=2
SCAN_INTERVAL_MIN=30
VNC_PORT=5900
CDP_PORT=9222
DISPLAY_NUM=99
```

---

## Development Notes

- **Build**: `go build ./cmd/scraper/` or `make build`
- **Run**: `./scraper` (loads `.env` automatically)
- **Frontend**: all in `internal/server/static/` — no build step, vanilla JS
- **Database migrations**: auto-run on startup in `store.go migrate()`
- **Chrome profiles**: `data/profiles/account_{id}/` — **never commit**
- **Agent tokens**: hashed with SHA-256, plaintext shown only once on creation
- **Authentication**: JWT access tokens (15min) + refresh tokens (7days) via HTTP-only cookie

---

## Current Git Status (as of 2026-04-24)

Uncommitted new files:
- `internal/ai/selector_healer.go` — GPT-4o Vision selector discovery
- `internal/browser/display.go` — Xvfb + x11vnc manager (Linux)
- `internal/server/agent_handlers.go` — Chrome Extension agent API
- `internal/server/cdp_view.go` — per-account CDP screencast (being refactored)
- `internal/server/vnc_proxy.go` — noVNC WebSocket proxy
- `internal/server/ws_hub.go` — Chrome Extension WebSocket hub
- `internal/store/agent_tokens.go` — agent token CRUD
- `internal/store/selector_cache.go` — selector cache CRUD

Uncommitted modifications:
- `internal/models/models.go` — added ExecutionMode to Job
- `internal/orchestrator/orchestrator.go` — added ProcessAgentScrapedPosts
- `internal/server/api.go` — added agent/browser/WS routes
- `internal/store/store.go` — added agent_tokens table, selector_cache, GetNextLocalJob
- `internal/queue/queue.go` — local jobs skip in-memory channel
- `internal/config/config.go` — VNC/CDP config vars
- `cmd/scraper/main.go` — wire SetPostProcessor
- `internal/server/static/index.html` — sidebar reorganized + Browser nav item

---

## If You Are Continuing This Work (Other Model)

**Priority 1** — Complete the Browser page:
1. `internal/workspace/workspace.go` (NEW) — Chrome per-account manager
2. Refactor `internal/server/cdp_view.go` — replace global hub with per-account `cdpHubs map[int64]*cdpViewHub`
3. `internal/server/workspace_handlers.go` (NEW) — REST + WS handlers
4. `internal/server/api.go` — add `workspace *workspace.Manager` and `cdpHubs` to Server struct, register new routes
5. `cmd/scraper/main.go` — instantiate `workspace.NewManager` and pass to `server.New`
6. `internal/server/static/index.html` — add Browser page section with account list + canvas
7. `internal/server/static/style.css` — add `btn-success`, `nav-section-label`, browser canvas styles
8. `internal/server/static/app.js` — add `loadBrowserPage()`, `browserStartAccount()`, `initAccountScreencast()`, WebSocket canvas renderer

**Priority 2** — Skills system:
1. `internal/skills/registry.go` — skill interface + registry
2. `internal/skills/scrape_group.go`, `post_comment.go`, etc.
3. `internal/ai/skill_router.go` — LLM routes natural language to skill
4. Wire telegram bot → skill router → skill executor

**Key constraint**: User does NOT want AI to generate or create images — only use real uploaded images from `data/images/` (via Telegram upload).

**Language**: User communicates in Vietnamese, UI text should be Vietnamese where appropriate.

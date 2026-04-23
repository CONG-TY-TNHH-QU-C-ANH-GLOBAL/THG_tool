---
description: THG Agentic Scraper v2 — full project architecture and module map
---

# THG Agentic Scraper v2 — Project Architecture

## Overview
An autonomous AI-powered lead generation and recruitment pipeline for Facebook. The system:
1. **Scrapes** Facebook groups for buyer-intent posts
2. **Classifies** posts as hot/warm/cold leads using AI (GPT)
3. **Auto-comments** on qualified leads with contextual sales messages + images
4. **Auto-posts** professional JD (Job Description) content to recruitment groups
5. **Manages** multi-account Facebook access with persistent Chrome profiles
6. **Orchestrates** everything via Telegram chatbot commands

## Tech Stack
- **Language**: Go 1.21+
- **Browser Automation**: chromedp (headless Chrome via CDP)
- **AI**: OpenAI API (gpt-4.1 for content, gpt-4o-mini for classification)
- **Database**: SQLite via go-sqlite3
- **Telegram**: go-telegram-bot-api
- **Deployment**: Docker + GitHub Actions → VPS (PM2)

## Directory Structure
```
THG_sale/
├── cmd/scraper/main.go          # Entry point — wires all components
├── internal/
│   ├── ai/                      # AI layer
│   │   ├── agent.go             # Telegram AI Agent (OpenAI Function Calling)
│   │   ├── classifier.go        # Post → Lead classification
│   │   ├── msggen.go            # Comment/Inbox/JD post generation
│   │   ├── group_scorer.go      # Group quality scoring
│   │   ├── pricer.go            # Price list extraction (Vision API)
│   │   └── selector.go          # AI-driven CSS selector discovery
│   ├── orchestrator/
│   │   ├── orchestrator.go      # Main coordinator (1800 lines)
│   │   └── recruitment_pipeline.go  # JD posting pipeline
│   ├── scraper/
│   │   ├── autocomment.go       # Facebook posting/commenting engine
│   │   ├── careerscraper.go     # Careers page scraper
│   │   └── imagescraper.go      # JD card image generator
│   ├── browser/                 # Chrome browser pool management
│   ├── queue/                   # Job queue with workers
│   ├── store/                   # SQLite database layer
│   ├── models/                  # Data models and types
│   ├── accounts/                # Multi-account management
│   ├── telegram/                # Telegram bot
│   ├── config/                  # Environment config
│   └── server/                  # REST API + Web UI
├── data/
│   ├── scraper.db               # SQLite database
│   ├── profiles/                # Chrome user profiles
│   └── images/careers/          # Generated JD card images
├── dist/                        # Web UI static files
└── .env                         # Environment variables
```

## Component Dependency Graph
```
Telegram Bot → AI Agent → Orchestrator → {Scraper, Queue, Store, AI}
                                            ↓
                                     AutoCommenter → Browser Pool → Chrome (CDP)
```

## Key Models (internal/models/)
| Model | Purpose |
|---|---|
| `Group` | Facebook group (URL, name, platform, niche) |
| `GroupQuality` | AI-scored quality (finalScore, category, blacklist) |
| `Post` | Scraped Facebook post (content, author, URL) |
| `Lead` | Classified lead (hot/warm/cold, niche, assigned staff) |
| `OutboundMessage` | Queued message to send (comment/inbox/group_post) |
| `CareerJob` | Job position from careers page |
| `Account` | Facebook account for automation |

## Configuration (.env)
```env
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini           # For scraping/classification (cheap)
OPENAI_COMMENT_MODEL=gpt-4.1       # For content generation (quality)
TELEGRAM_BOT_TOKEN=...
TELEGRAM_ADMIN_CHAT_ID=...
CHROME_PATH=C:\Program Files\Google\Chrome\Application\chrome.exe
PROFILE_DIR=data/profiles
WEB_PORT=8080
SCAN_INTERVAL_MIN=60
MAX_WORKERS=2
```

## Startup Flow (cmd/scraper/main.go)
```
1. Load .env → config.Load()
2. store.New() → SQLite init + migrations
3. browser.NewPool() → Chrome contexts
4. queue.New() → Job queue with workers
5. ai.NewClassifier() → gpt-4o-mini
6. ai.NewMessageGenerator() → gpt-4.1
7. ai.NewAgent() → gpt-4.1 (Telegram AI)
8. accounts.NewManager() → Multi-account
9. orchestrator.New() → Wire everything
10. telegram.Bot.Start() → Listen for commands
11. server.New().Start() → REST API + Web UI
```

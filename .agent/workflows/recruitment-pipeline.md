---
description: How the recruitment pipeline works — from job posting to candidate processing
---

# Recruitment Pipeline

## Overview
The recruitment pipeline automates the full cycle:
1. Scrape careers page → store jobs in DB
2. Generate JD card images for each job
3. AI writes professional recruitment posts
4. Auto-post to domain-matched Facebook groups
5. Scan posted JDs for candidate comments
6. Process candidates as leads in "tuyen_dung" niche

## File: `internal/orchestrator/recruitment_pipeline.go`

### Key Functions

#### `PostJDsToExistingGroups(ctx, jobs)`
Creates and posts JD content to Facebook groups.
- **Input**: Pre-filtered `[]models.CareerJob` (caller handles position filtering)
- **Flow**:
  1. Get default Facebook account
  2. Load ALL scored groups from DB
  3. Normalize group categories: AI categories → standard domains
  4. For each job: find domain-matched groups → AI generate post → queue as `group_post`
  5. Trigger AutoComment queue to execute posting

#### Domain Mapping
| Job Domain | Groups Category |
|---|---|
| `tech` | IT, data, AI, developer groups |
| `sales` | E-commerce, sales, marketing groups |
| `ops` | Logistics, operations, XNK groups |
| `finance` | Accounting, finance groups |
| `general` | HR/recruitment groups (fallback) |

#### `normalizeGroupCategory(cat)`
Maps AI-returned categories to standard domain keys:
```
"technology"/"it"/"data_science"/"ai" → "tech"
"ecommerce"/"sales"/"marketing" → "sales"
"logistics"/"shipping"/"operations" → "ops"
"accounting"/"finance"/"banking" → "finance"
"hr_recruitment"/"job_portal" → "general"
```

### Anti-Spam Rules
- Max 2 posts per group per week
- Skip low-priority jobs
- Groups with `FinalScore < 0.1` excluded
- Blacklisted groups excluded

## Group Quality Scoring (`internal/ai/group_scorer.go`)
- Uses GPT to evaluate groups on 4 dimensions
- Stores: `FinalScore`, `Category`, `Blacklist`, `WeeklyPostCount`
- Score threshold for posting: 0.1 (low, because name-only context)

## JD Card Images (`internal/scraper/imagescraper.go`)
- Generates visual JD cards for each career position
- Stored in `data/images/careers/`
- Attached to group posts automatically

## Telegram Triggers
User messages on Telegram → AI Agent → Function calls:
| User Says | Agent Calls | What Happens |
|---|---|---|
| "đăng bài tuyển dụng" | `post_jds_to_groups({})` | Post ALL positions |
| "đăng bài Accountant, Sales" | `post_jds_to_groups({positions:"Accountant, Sales"})` | Post ONLY specified positions |
| "quét bài đã đăng" | `scan_own_jd_posts({})` | Scan comments on posted JDs |
| "chạy pipeline tuyển dụng" | `full_recruitment_pipeline({})` | Full cycle: scrape→score→post |

## Candidate Processing
When someone comments on a posted JD:
1. `ScanOwnJDPosts()` finds posted JDs in DB
2. Scraper navigates to each post → extracts comments
3. Each commenter becomes a lead in `tuyen_dung` niche
4. HR Agent generates @reply to engage the candidate

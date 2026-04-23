---
description: How the Facebook automation engine works — scraping, commenting, posting, inbox
---

# Facebook Automation Engine

## Browser Management
- **Pool**: `internal/browser/pool.go` manages N Chrome contexts with persistent profiles
- **Profiles**: Stored in `data/profiles/` — each account has its own Chrome user data dir
- **Acquisition**: `pool.Acquire(timeout)` → returns a browser context; `pool.Release(ctx)` returns it
- **CDP**: Uses `chromedp` library for all browser interactions (no Selenium/Puppeteer)

## AutoCommenter (internal/scraper/autocomment.go)
The central engine for ALL Facebook interactions. Routes by `msg.Type`:

### Message Types
| Type | Method | What it does |
|---|---|---|
| `"comment"` (default) | `PostComment()` or `PostCommentWithImage()` | Comment on a lead's post |
| `"comment_reply"` | `PostCommentReply()` | @reply to a specific commenter |
| `"group_post"` | `PostToGroup()` | Create a new post in a group |

### PostToGroup Flow (JD Posting)
```
1. Navigate to www.facebook.com/groups/{slug}
2. JS finds "Bạn viết gì đi..." text → returns X,Y coordinates
3. Native DispatchMouseEvent click at those coordinates (triggers React)
4. Wait for dialog (div[role="dialog"]) with contenteditable/textbox
5. Type content via ClipboardEvent paste → fallback CDP InsertText
6. Upload image via dialog input[type="file"]
7. Find "Đăng" button text → native mouse click to submit
8. Wait 8s for Facebook to process
```

### PostCommentWithImage Flow
```
1. Navigate to post URL
2. Find comment input (aria-label="Write a comment" / "Viết bình luận")
3. Attach image FIRST (photo click → file input)
4. Type text via ClipboardEvent paste → execCommand → CDP InsertText
5. Submit via Post/Đăng button → fallback CDP Enter key
6. Verify input cleared (7s wait)
```

### Key Technical Details
- **React Compatibility**: Facebook uses React — must use `ClipboardEvent` for paste, `DispatchMouseEvent` for clicks. JS `.click()` does NOT trigger React handlers!
- **Article Scoping**: `articleScopeJS(postID)` narrows DOM search to specific post
- **Typing Fallback Chain**: ClipboardEvent paste → `execCommand('insertText')` → CDP `input.InsertText`
- **Submit Fallback**: Button click → CDP Enter key dispatch

## Scraping Pipeline (orchestrator.handleScrapePostsJob)
```
1. Get active groups from DB
2. For each group: navigate → scroll → extract posts
3. Deduplicate posts (compound key: author + content hash)
4. AI classify: hot/warm/cold leads
5. For qualified leads: generate AI comment → queue as OutboundMessage
6. Trigger AutoComment queue job
```

## Anti-Spam Protections
- **Weekly post limit**: Max 2 posts per group per week (`WeeklyPostCount`)
- **Jitter delays**: Random sleep between actions (anti-bot detection)
- **Batch size**: Max 5 comments per queue job (10 min timeout)
- **Group blacklist**: Low-quality/logistics groups auto-blacklisted

## Troubleshooting Facebook Automation

### Common Failures
| Error | Cause | Fix |
|---|---|---|
| `account not logged in` | Chrome profile cookies expired | Re-login manually in Chrome profile |
| `could not find compose area` | Page DOM changed or not logged in | Check page title in logs — should show group name |
| `dialog not ready` | Compose click didn't open dialog | Check if user has posting permission in group |
| `no_submit_btn` | "Đăng" button not found | FB may have changed DOM — update submit JS |
| `empty_after_paste` | ClipboardEvent failed | CDP InsertText fallback should handle this |
| `context canceled` | Server shutdown during operation | Normal if you ctrl+C'd the server |

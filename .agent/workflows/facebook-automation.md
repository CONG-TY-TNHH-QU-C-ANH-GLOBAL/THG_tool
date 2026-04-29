---
description: How the Facebook automation engine works in production
---

# Facebook Automation Engine

## Browser Management

- **Workspace**: `internal/workspace` owns one persistent Chrome container/profile per Facebook account.
- **Profile path**: `data/profiles/account_<id>/` is durable and must not be deleted during deploys.
- **Visibility**: dashboard Browser view streams the real account browser via `/ws/screen/:id` and can still expose per-account VNC at `/ws/vnc/:id` when needed.
- **Automation**: worker/runtime code attaches to the running workspace CDP port instead of launching a hidden local browser pool.

## Prompt-Driven Crawler

Production crawling starts from a user prompt in Telegram or dashboard chat:

```
User prompt
  -> AI Agent action
  -> scheduler job (Intent: facebook_crawl / web_crawl / lead_gen)
  -> worker handler
  -> workspace runtime
  -> classifier
  -> leads / outputs
```

The crawler should not assume one fixed set of groups. The prompt supplies the
target URL/search query, intent, product context, region, and limits. `/scan_all`
and configured-group loops are disabled as product behavior unless they are
explicitly reintroduced as a scheduled campaign feature.

## Job Payload Contract

Use open crawler tasks:

- `Intent`: `facebook_crawl`, `web_crawl`, or `lead_gen`
- `Source.Type`: `facebook_group`, `facebook_post`, `facebook_search`, or `web_url`
- `Source.URL` / query: supplied by the user or parsed from prompt
- `Keywords`: derived from prompt text
- `OutputSchema`: `open_crawler_v1`

`scrape_group` is only a compatibility alias for old jobs and should stay gated
behind `ENABLE_LEGACY_SCRAPE_GROUP=true`.

## Classification Contract

Every candidate must be filtered against the current business context before it
becomes a lead. The universal classifier should reject uncertain or mismatched
items instead of inflating lead volume. Deterministic scoring can be used as a
fallback when the model call fails, but it is not the primary product promise.

## Facebook Actions

Posting, commenting, and inbox actions must reuse the selected workspace account:

- verify the workspace is running
- verify Facebook login/session ownership (`c_user`) before saving status
- attach automation to the visible Chrome session
- log action result and failure reason
- avoid mixing data across Facebook accounts in the same workspace/org

## Troubleshooting

| Symptom | Likely cause | Check |
|---|---|---|
| Browser tab blank | workspace container not running or CDP port not ready | `/api/browser/workspaces`, `/ws/screen/:id` |
| Account mismatch warning | Chrome profile logged into a different Facebook user | `c_user` validation in workspace set-logged-in flow |
| Crawler returns no leads | classifier rejected candidates or prompt lacks target/context | AI response, job logs, business context |
| Job stays pending | worker not running or scheduler claim failed | `cmd/worker` logs and `/api/jobs` |
| Facebook checkpoint | account needs human action | Browser view for that account |

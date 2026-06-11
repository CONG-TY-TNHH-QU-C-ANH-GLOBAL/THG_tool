# Telegram Event Wiring (crawler + comment → channel)

> Track: **Omnichannel / Telegram**. Wires REAL automation events to the per-org Telegram channel
> destinations via `control.Service.NotifyEvent`. Until this PR the notification infra existed but
> had **zero callers** — so "crawl finished" and comment outcomes never reached the channel.

## Root cause (diagnosed)
`control.NotifyEvent` had no production callers, and the only wired Telegram path (`telegramNotify`)
sent to the **global admin chat** via the global bot — not the per-org channel destinations. So a
connected channel received nothing on crawl/comment.

## What was wired

### lead_created (crawler)
Single choke point: `leadingest.IngestPost` gained an optional `OnLeadCreated func(LeadEvent)` hook,
fired once per NEW lead (best-effort, never blocks ingest). Both crawl paths set it:
- **Worker** (`cmd/worker/main.go`) — the background crawl handler. Builds a `control.Service` and
  wires `h.SetLeadNotifier(...)`. **Also sets `mainStore.SetEncryptionKey(ENCRYPTION_KEY)`** so the
  worker can DECRYPT each org's bot token (without it, per-org delivery silently fails).
- **Server/extension crawl** (`internal/server/agent/crawl.go`) — sets `OnLeadCreated` from the
  shared `tgControl`.

### comment / inbox / post outcome (agent)
At the connector outcome finalize (`internal/server/agent/outbox_agent.go`, first-win path), after
the existing dashboard notification, emit a per-org channel notification via `tgControl.NotifyAgentAction`.
`agentEventType` maps action+outcome → `comment_verified` / `comment_unverified` (submitted, pending
verify) / `comment_failed`, `inbox_sent`/`inbox_failed`, `post_submitted`/`post_failed`.

### Shared service
`control.Service` is built ONCE in `internal/server/router.go` and shared by: the connector outcome
emitter (ConnectorRoutes), the dashboard, the REST integrations API, and the webhook runtime.

### Rich payload + rendering (notification quality)
`control` exposes `NotifyLead(LeadNotice)` + `NotifyAction(ActionNotice)`. The CALLER provides
resolved business data (workspace name via `store.GetOrganization`, agent account name via
`identities.GetAccount` → `Facebook <FBDisplayName>`, post URL, raw excerpt); `control` then:
- **sanitizes** the excerpt (`SanitizeExcerpt`): collapse whitespace, drop consecutive repeated
  tokens, REJECT channel-spam-only content (the "Facebook Facebook Facebook…" garbage → ""), cap
  300 runes. Empty → renderer shows "Chưa có nội dung tóm tắt. Mở bài viết để xem chi tiết."
- **builds URLs** centrally (`dashboardLeadURL`, `outboxURL`) — empty base → empty link → the
  renderer **omits the line** (never a dangling "Mở dashboard:").
- **renders** plain-text, mobile-readable messages (`render.Lead` / `render.Action`) that omit any
  empty field/link. `actionPresentation` maps each event type → header/status/hint/failure-flag;
  `comment_unverified` reads informational (ℹ️ + manual-check hint), not a failure. Future
  `post_*`/`inbox_*` headers are already in the table.

`leadingest.LeadEvent` carries `AuthorName/PostURL/Excerpt(raw)/Reason/SourceType/GroupFBID`.

## Delivery path
emitter → `control.NotifyEvent(orgID, eventType, channel, message)` → resolves the org's bot
(`resolveBot`) → for each active destination subscribed to that event type + channel filter →
`bot.Send(chat_id, message)` → `RecordDelivery` + audit. A destination only receives events it has
opted into (per-destination `event_types`).

## Deploy notes
- **Worker** needs `ENCRYPTION_KEY` (same value as the server) + `TELEGRAM_NOTIFY_ENABLED` +
  optionally `APP_BASE_URL` (for dashboard links). Without `ENCRYPTION_KEY` the worker cannot
  decrypt org bot tokens → lead notifications silently won't send.
- A destination must be subscribed to the event types you expect (default on connect = all).

## Tests
- `control/sanitize_test.go`: spam-only → ""; blank → ""; content+trailing-spam cleaned + whitespace
  collapsed; long excerpt trimmed with ellipsis.
- `control/events_test.go`: `NotifyLead` renders workspace/source/author/excerpt/post-URL/dashboard;
  garbage excerpt → fallback (no repeated Facebook); empty base URL hides the dashboard line; org 0
  no-op. `NotifyAction` verified→success header, unverified→informational (not "Thất bại") + hint,
  failed→reason + hint; unsubscribed/invalid → nothing.
- Build-verified wiring across worker + server + leadingest + agent.

## Confirmations
No source file >200 lines · best-effort (a notification failure never affects crawl/comment) · no
direct `/comment` execution · no Telegram→Extension path · Facebook hot path / composer untouched ·
no Vision. NOT wired here: connector_offline (needs a state-change detector — separate follow-up).

# Telegram Notification Destinations (channel-first)

> Track: **Omnichannel / Telegram**. Product correction: Telegram is a **workspace notification
> channel** for automation operations. The **bot is the transport/sender**, not the product. The
> DM `/bind` flow (PR-1/runtime) is kept as an **optional secondary** path, not the primary UX.
> Builds on `specs/TELEGRAM_BOT_RUNTIME.md`. **This PR = BACKEND**; the channel-first UI follows.

## Acceptance answers

- **Is Telegram channel the primary setup path?** Yes. The new `telegram_destinations` model is the
  primary delivery target; `telegram_bindings` (personal DM) is retained but secondary.
- **How does an admin connect a channel?** Two methods (simplest reliable per Bot API):
  - **Public channel** — admin adds the bot as admin, enters `@username`; the backend calls
    `sendMessage("@username")` (one verified call) and stores the `chat.id`/`title`/`username`
    Telegram returns. Synchronous, no webhook race.
  - **Private channel** — admin requests a one-time **connect code**, adds the bot as admin, and
    posts `/connect <code>` in the channel; the bot (an admin) receives the `channel_post` update,
    matches the code → org, and stores the channel. Tenant-safe (code ties to org).
- **How do we store chat_id/title/status?** `telegram_destinations` (migration 0015): `org_id`,
  `destination_type` (channel|group|personal_dm), `chat_id`, `title`, `username`, `status`
  (active|disabled|needs_attention), `event_types` (JSON), `channel_filter`, `delivery_mode`,
  `connected_by_user_id`, `last_delivery_at`, `last_error`, timestamps, `revoked_at`. Unique
  (org_id, chat_id) — reconnect updates in place. `chat_id` is never exposed to the UI.
- **How does a crawler lead / agent-action notification reach the channel?** The emitter calls
  `control.Service.NotifyEvent(orgID, eventType, channel, message)` — it NEVER calls Telegram
  directly. The service resolves the org's deliverable destinations, filters each by its subscribed
  `event_types` + `channel_filter`, sends via the bot, records delivery (`RecordDelivery`), and
  audits. `render.LeadCreated/AgentComment/Failure` build the message text.
- **What remains of personal DM binding?** `/start /help /bind /status /unbind` + `telegram_bindings`
  stay for optional per-user DM recipients + command auth. They do NOT gate channel delivery — a
  workspace channel works with zero personal `/bind`.
- **UI copy changes (next PR):** primary card "Kết nối Telegram Channel" / "Kênh thông báo"; a
  separate lower section "Người dùng liên kết cá nhân" for optional DM. No implication that every
  user must DM the bot.
- **Tests proving tenant-safe channel delivery:** see below.

## Backend shape

- **Store** `internal/store/telegram/destinations*.go`: `UpsertDestination` (create / reconnect),
  `ListDestinations`, `ListActiveDestinations` (deliverable = not `disabled`), `GetDestination`,
  `UpdateDestinationPreferences`, `SetDestinationStatus`, `DisableDestination`, `RecordDelivery`,
  `CountDestinations`. All org-scoped; unique (org, chat).
- **Domain** `internal/telegram/control`: `Bot` interface gains `Resolve(ref,text)→(chatID,title,
  username)`; `destinations.go` = `ConnectPublicChannel`, `HandleChannelPost`, `TestDestination`,
  `SetDestinationPreferences`, `DisableDestination`, `ListDestinations`; `notify_event.go` =
  `NotifyEvent` routing; `policy.go` adds `EventTypes` allow-list + `IsValidEventType` +
  destination audit-event names. Single source of truth.
- **Client** `internal/telegram/client`: `Resolve(ref,text)` via `sendMessage` (returns the
  resolved chat); stays import-free (primitive returns); token never logged.
- **Webhook** `internal/server/telegram`: now also parses `channel_post` → `HandleChannelPost`.
- **REST** `internal/server/integrations/telegram_destinations.go`:
  `GET/POST /destinations`, `DELETE /destinations/:id`, `POST /destinations/:id/test`,
  `PUT /destinations/:id/preferences`; status adds `active_destinations`. Bindings endpoints kept.

## Delivery semantics
A send failure marks a destination `needs_attention` (soft warning surfaced in UI) but it KEEPS
receiving — only `disabled` stops delivery. A later successful send flips it back to `active`.

## Event types (subscribable per destination, channel-neutral)
Lead: `lead_created` `lead_assigned` `lead_ready_for_review` · Agent: `comment_submitted`
`comment_verified` `comment_unverified` `comment_failed` `post_submitted` `post_failed`
`inbox_sent` `inbox_failed` · System: `connector_offline` `account_attention` `automation_paused`
`gate1_failure_spike` `submitted_unverified_spike` `circuit_breaker_triggered`. Filters: all /
facebook / taobao / 1688.

## Tests (shipped)
- store `destinations_test.go`: CRUD, reconnect-in-place, **tenant isolation** (same chat id across
  orgs = separate rows; org A never sees org B), prefs round-trip, delivery → needs_attention,
  disable removes from active/count.
- control `destinations_test.go`: connect public (Resolve ok → stored; fail → `resolve_failed`,
  nothing stored), private channel_post connect (code → destination; wrong code → nothing),
  test-destination (ok / failing→needs_attention), **NotifyEvent routing** (event-type filter,
  channel filter, invalid-type reject, notify-disabled, **cross-org isolation**), destinations +
  DM bindings coexist.

## NOT in this PR (next)
- Channel-first UI (cards/setup/preferences/test) + relabelled DM section.
- Wiring real emitters (crawler/comment/posting/inbox/connector) to `NotifyEvent` — the service +
  message builders are ready; call-sites are the follow-up.

## Confirmations
No source file >200 lines · no direct Telegram `/comment` execution (control-plane only;
`TELEGRAM_ACTIONS_ENABLED` off) · no Telegram → Extension path · bot token never exposed · no
duplicated domain logic (event/filter allow-lists + audit names live only in `control`).

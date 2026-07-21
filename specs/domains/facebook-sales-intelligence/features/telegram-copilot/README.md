# Feature: telegram-copilot

Tenant-scoped Telegram bounded context: per-org bot credentials, webhook
runtime, channel-first notification destinations, and real automation-event
wiring (`internal/telegram/`, `internal/server/telegram`, migrations
0013–0016). Telegram is a workspace notification channel; the bot is the
transport, and action execution stays gated OFF by default
(`TELEGRAM_ACTIONS_ENABLED`).

- [technical.md](technical.md) — bounded-context runtime architecture
  (store → control → integrations/server; webhook runtime; legacy long-poll
  bot retired). Implementation state: **backed**.
- [implementation/integration-ui.md](implementation/integration-ui.md) —
  tenant-scoped setup/manage/revoke backend + UI (PR-1).
- [implementation/channel-destinations.md](implementation/channel-destinations.md)
  — channel-first destinations backend (migration 0015).
- [implementation/event-wiring.md](implementation/event-wiring.md) — real
  crawler/comment events → per-org channels via `control.Service.NotifyEvent`.
- [implementation/per-org-bot.md](implementation/per-org-bot.md) — per-org
  encrypted bot credentials (migration 0016, AES-GCM, token never logged).

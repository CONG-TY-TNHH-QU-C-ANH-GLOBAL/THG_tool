# Telegram Bot Runtime + Bounded-Context Architecture

> Track: **Omnichannel / Telegram**. This PR (1) consolidates the Telegram backend into one
> bounded context with a single domain/service layer, (2) **retires** the legacy long-poll
> agent-bot, and (3) adds the tenant-scoped **webhook** runtime. Builds on PR-1 (control-plane
> store + REST API, `specs/TELEGRAM_INTEGRATION_UI.md`).

## Architecture review outcome — the boundary

The pre-existing Telegram code had grown in three places with **no shared domain layer** (rules
lived inside the REST handlers) plus a 634-line single-org long-poll prototype. The blocking
finding: **Telegram forbids `getUpdates` (long-poll) and a webhook on the same token at once**, so
the legacy bot and the new webhook cannot coexist. Founder decision: **retire the legacy bot;
webhook is the single runtime.**

| Layer | Package | Responsibility |
|---|---|---|
| **A. Store** | `internal/store/telegram` | DB CRUD only — settings, bind codes, bindings, alert prefs, audit, `chat_id`, `last_command_at`, runtime lookups. No HTTP/Telegram API. |
| **B. Domain/service** (single source of truth) | `internal/telegram/control` | command registry, ACTIONS_ENABLED policy + denied text, audit-event names, alert-type & channel-filter allow-lists, ownership rules, `Sender` interface, binding/command/notification services. No HTTP, no token. |
| **C. REST API** | `internal/server/integrations` | thin tenant/role-scoped settings handlers; calls B for allow-lists, audit names, test-notification. Re-implements nothing. |
| **D. Webhook runtime** | `internal/server/telegram` | thin `POST /api/telegram/webhook`; secret check + update parse → `control.HandleMessage`. No business logic. |
| **E. Bot client** | `internal/telegram/client` | HTTP `sendMessage`; implements `control.Sender`; token never logged/returned/in errors. |
| **F. Renderers** | `internal/telegram/render` | vi-primary reply text; leaf package, no logic, no secrets. |

**Dependency direction (acyclic, proven by `go build`):** store → control → {integrations,
server/telegram}; control → render; client/render are leaves. The webhook route is **public**
(Telegram sends no JWT) — authenticity is the webhook secret.

## Legacy retirement (controlled)

- `internal/telegram/bot.go` (long-poll, telebot, hardcoded org=1) **deleted**; the `telebot.v3`
  dependency removed (`go mod tidy`). `main.go` no longer constructs/starts it.
- The **system notifier** now sends to the admin chat via the new `client` (not `bot.Notify`).
- **Single runtime confirmed**: one webhook runtime · one bot client · one command service · one
  notification service · one shared `control` package. No `org_id=1` remains in the active runtime.

### Backlog — legacy org-1 agent-console capabilities (NOT ported in this PR)
`/scan` · `/price` · `/add` · `/results` · `/stats` · `/groups` · free-text → AI agent · photo →
price extraction. If still useful, migrate them later into the tenant-scoped control-plane (each
must become org/binding-scoped, RBAC + audit checked, and routed through `ActionContext` —
**never** a direct execution path). Tracked in [[project_omnichannel_telegram_track]].

## Runtime

- **Endpoint:** `POST /api/telegram/webhook` (public; `X-Telegram-Bot-Api-Secret-Token` validated
  when `TELEGRAM_WEBHOOK_SECRET` is set; always returns 200 on a parsed update so Telegram does not
  retry benign messages).
- **Commands:** `/start`, `/help`, `/bind <code>`, `/status`, `/unbind`; unknown → safe help;
  `/comment`·`/send`·`/auto_comment`·… → **denied** (no execution path exists; `ACTIONS_ENABLED`
  off).
- **Bind consume:** normalise code (trim+upper) → validate (exists, unused, unexpired, single-use)
  → create active binding storing `telegram_user_id`/`chat_id`/`username`/`display_name` → audit
  `bind_success`/`bind_failed`.
- **last_command_at:** stamped for a bound account on every command; surfaced in the bindings REST
  response (`last_command_at`).
- **Notifications:** `Service.NotifyBoundUsers(orgID, alertType, channel, message)` respects
  `TELEGRAM_NOTIFY_ENABLED`, the org's `alerts_enabled`, per-type opt-in, and channel filter; sends
  to active alert-recipient bindings; audits `notification_sent`/`notification_failed`.
  `TestNotify(orgID, userID)` backs the REST test-notification button (real send + audit).

## Config / feature flags
`TELEGRAM_BOT_TOKEN` (webhook runtime + client) · `TELEGRAM_NOTIFY_ENABLED` (default true) ·
`TELEGRAM_ACTIONS_ENABLED` (**default false — must stay off**) · `TELEGRAM_WEBHOOK_SECRET`
(optional; recommended in production).

## Webhook setup (operator)
1. Deploy with `TELEGRAM_BOT_TOKEN` set and a strong `TELEGRAM_WEBHOOK_SECRET`.
2. Register the webhook with Telegram (once):
   `curl -F "url=https://<host>/api/telegram/webhook" -F "secret_token=$TELEGRAM_WEBHOOK_SECRET" https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/setWebhook`
3. Ensure no long-poll consumer runs on the same token (the legacy bot is retired, so this holds).
4. In the web app: Settings → Integrations → Telegram → Enable → Generate code → `/bind <code>`.

## Deployment
Standard backend deploy (migration `0014` adds `telegram_bindings.chat_id`). No frontend change in
this PR. Then run the `setWebhook` call above.

## Rollback
- Drop the webhook: `deleteWebhook` on the token; the runtime simply receives nothing.
- Revert this commit to restore the prior state. (The legacy long-poll bot is intentionally gone;
  rolling back the runtime does not auto-restore it — restore from git history only if required.)
- Migration `0014` is additive (a defaulted column); no rollback needed for data integrity.

## Confirmations
No direct `/comment` or execution path · `TELEGRAM_ACTIONS_ENABLED=false` default · no Telegram →
Extension path · no bypass of PolicyGate/WorkQueue/Outbox/Ledger (control-plane only; no outbound
execution) · Facebook hot path untouched · no source file >200 lines · bot token never exposed ·
no duplicated Telegram domain logic (allow-lists, audit names, binding/permission rules live only
in `control`).

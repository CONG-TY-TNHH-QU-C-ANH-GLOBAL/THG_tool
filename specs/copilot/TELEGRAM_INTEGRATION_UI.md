# Telegram Integration — Enterprise SaaS Setup (Backend + UI)

> Track: **Omnichannel Sales Copilot / Telegram** (see
> `specs/OMNICHANNEL_SALES_COPILOT_TELEGRAM_TRACK.md`). Goal: admins set up, manage, monitor, and
> revoke the Telegram integration from the web app — a clean tenant-scoped SaaS experience, not a
> command-line bot. **Read-only / control-plane only**: action EXECUTION (`/comment` etc.) is
> gated by `TELEGRAM_ACTIONS_ENABLED` and stays OFF by default.
>
> **PR-1 (this PR) = BACKEND, shipped + verified.** PR-2 = the Next.js UI against this contract.

## Feature flags (process-level, default-safe)

| Env | Default | Meaning |
|---|---|---|
| `TELEGRAM_BOT_ENABLED` | `false` | the bot polling/interface is enabled |
| `TELEGRAM_NOTIFY_ENABLED` | `true` | push notifications/alerts may be sent |
| `TELEGRAM_ACTIONS_ENABLED` | `false` | **must stay false** — no Telegram-initiated action execution |
| `TELEGRAM_BOT_TOKEN` | `""` | bot token; presence ⇒ `bot_configured` (value never returned by any API) |

Wired in `internal/config/config.go` → mirrored into `server.Config` → surfaced to handlers as a
`Flags` struct so handlers never import config.

## Tables (migration `0013_telegram_integration__sqlite.up.sql`)

All org-scoped; revocation/audit are append-only.

- `telegram_settings` — per-org `enabled` + `bot_username` + webhook health (`webhook_last_at`,
  `webhook_last_err`).
- `telegram_bind_codes` — one-time pairing codes (`code` unique, `expires_at`, `used`).
- `telegram_bindings` — user ↔ `telegram_user_id` (`display_name`, `role`, `alert_recipient`,
  `status` = active|revoked). Revoked, never hard-deleted.
- `telegram_alert_prefs` — org-level `alerts_enabled` + channel-neutral `channel_filter`
  (all|facebook|taobao|1688) + `alert_types` JSON.
- `telegram_audit` — append-only control-plane events.

Store domain: `internal/store/telegram/` (`Store.Telegram()` accessor; zero cross-domain writes).

## Endpoints (`/api/settings/integrations/telegram`, tenant-scoped)

| Method · Path | Role | Purpose |
|---|---|---|
| `GET /status` | member | headline state (not_connected/connected/needs_attention), counts, flags, channel list. **No token ever returned.** |
| `POST /enable` · `POST /disable` | admin | toggle integration (audited) |
| `POST /bind-codes` | member | issue a one-time code for the caller (binds self); returns `deep_link` `t.me/<bot>?start=<code>` + TTL |
| `GET /bindings` | member→own / admin→all | list bindings (server-side role scoping, `can_manage_all` flag) |
| `DELETE /bindings/:id` | member→own / admin→any | revoke (ownership-checked, audited) |
| `POST /test-notification` | member | queue a test to caller's binding (400 if notify disabled / no binding) |
| `GET /alerts` · `PUT /alerts` | member read / admin write | alert prefs; validates channel_filter + alert_types allow-lists |
| `GET /audit` | admin | recent control-plane events |

RBAC: `adminOnly` middleware gates org-level mutations + full bindings/audit; mixed routes scope
by role/ownership **inside** the handler (`canViewAllBindings` = admin or platform owner). org_id /
user_id / role come from `c.Locals` — every query filters by org_id (no cross-tenant reads).

Handlers: `internal/server/integrations/` (`routes.go`, `telegram_status.go`, `telegram_bind.go`,
`telegram_bindings.go`, `telegram_alerts.go`). Registered in `internal/server/router.go`.

## Safety guarantees

- Bot token value is never returned by any endpoint — only a `bot_configured` boolean.
- No `/comment`-style execution endpoint exists; `actions_enabled` is surfaced (false) for the UI
  to display the "execution disabled" notice.
- Channel-neutral throughout: `channel_filter` + the `channels[]` status list model
  Facebook/Taobao/1688 with one shape; no Facebook-only assumptions.
- Bind-code expiry is compared against a Go-encoded `time.Now()` (not SQLite `CURRENT_TIMESTAMP`)
  to avoid the mixed-encoding bug that would let codes never expire.

## Tests (shipped)

- `internal/store/telegram/telegram_test.go` — DB-backed (storetest template): settings/alerts
  upsert, bind-code consume + single-use + **expiry**, bindings list/scope/revoke/counts with
  cross-org isolation, audit append + cross-org isolation.
- `internal/server/integrations/telegram_internal_test.go` — pure: `computeStatus` matrix,
  `canViewAllBindings` role matrix, validation allow-lists.

## PR-2 — UI (next, against this exact contract)

`services/telegramIntegrationApi.ts` + `components/telegram/` (StatusCard, SetupGuide,
BindCodeCard, BindingsTable, AlertPreferences, AuditPanel, SafetyNotice) under
Settings → Integrations → Telegram; each component ≤200 lines; empty/connected/needs-attention/
error states; role-gated admin UI; i18n (vi primary). See the requesting brief for the full UI
spec.

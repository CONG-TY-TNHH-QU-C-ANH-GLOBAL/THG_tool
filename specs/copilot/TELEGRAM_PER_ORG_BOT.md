# Per-Org Telegram Bot Credentials (multi-tenant)

> Track: **Omnichannel / Telegram**. SaaS correction: the Telegram bot token is **org/workspace-
> scoped**, not a hardcoded global platform token. Each workspace connects its OWN bot (BotFather)
> and channel delivery uses THAT bot's token. The token is a **customer secret** — encrypted at
> rest, never returned to the UI, never logged. **This PR = BACKEND**; the "connect your bot" UI
> (Step 1) follows.

## Root cause this fixes
The admin added bot **@THG_Sale** as admin of channel **@THG_Sale_Lead**, but the backend sent
`sendMessage("@THG_Sale_Lead")` with a *different* global `TELEGRAM_BOT_TOKEN` → Telegram 400/403
→ `resolve_failed`. Now the org saves its **@THG_Sale** token (Step 1) and channel connect/delivery
uses that token — so the bot that is actually an admin is the one that sends.

## Acceptance answers
- **Org/workspace-scoped?** Yes. `telegram_bot_credentials` (one row per `org_id`). Channel
  delivery resolves the org's own token; no global token required for tenant delivery.
- **Encryption?** `auth.Encrypt` (AES-256-GCM via the app `ENCRYPTION_KEY`) → `token_encrypted`.
  Decrypted ONLY inside the send/verify path (`GetDecryptedBotToken`, internal-only). Empty key =
  no-op (dev), same as cookies.
- **UI after saving token (next PR):** "Đã kết nối bot @your_bot" + `bot_configured`, username,
  display name, `token_last4`, `status`, `last_verified_at`. The token is never shown again.
  Backend `GET /bot` returns exactly these safe fields.
- **How channel connect knows which bot:** `Service.resolveBot(orgID)` → org's decrypted token →
  `BotFactory(token)` → client. `ConnectPublicChannel` / `TestDestination` / `NotifyEvent` all use
  the per-org bot.
- **Webhook resolves bot/org safely:** per-workspace webhook is **PENDING** (documented below).
  The current shared webhook belongs to the platform/dev bot (`GlobalToken`). Channel delivery (the
  fix) needs no webhook — it is synchronous `sendMessage` with the org token.
- **Is global `TELEGRAM_BOT_TOKEN` still used?** Only as (a) the platform/dev **webhook** bot (DM
  commands + private channel_post connect for the dev bot) and (b) an OPTIONAL tenant fallback
  behind `TELEGRAM_ALLOW_GLOBAL_FALLBACK` (**default false**). Not for tenant delivery by default.
- **Tokens/chat_ids hidden from UI?** Yes. Token never returned (only `token_last4`); `chat_id` is
  structurally absent from API types (`json:"-"`).
- **Errors sanitized + specific?** Yes — `bot_token_missing`, `bot_token_invalid`,
  `channel_not_found_or_username_invalid`, `bot_not_channel_admin`, `bot_lacks_post_permission`,
  `telegram_api_error`, `network_error` (mapped from Telegram `error_code` + description;
  description never echoed verbatim).
- **All files <200 lines?** Yes (largest control/service.go 138).

## Schema — migration 0016
`telegram_bot_credentials(org_id UNIQUE, bot_id, bot_username, bot_display_name, token_encrypted,
token_last4, status active|invalid|revoked|needs_attention, created_by_user_id, last_verified_at,
last_error, …, revoked_at)`.

## Backend shape
- **Store** `internal/store/telegram/bot_credentials.go` (+`encKey` on the telegram store):
  `UpsertBotCredential` (encrypts), `GetBotCredential` (metadata, no token), `GetDecryptedBotToken`
  (internal send/verify path only), `SetBotStatus`, `RevokeBotCredential` (wipes token).
- **Domain** `internal/telegram/control`: `Bot` interface gains `GetMe()` + `Resolve()→SendResult`
  (carries Telegram error code/desc). `BotFactory(token)` replaces the single injected bot.
  `resolveBot(orgID)` (org token → fallback only if allowed), `globalBot()` (platform/webhook bot).
  `bot.go`: `SaveBotToken` (getMe-verify → encrypt → store), `VerifyBot`, `BotStatus`, `RevokeBot`,
  `classifyTelegramError`.
- **Client** `internal/telegram/client`: one client per token; `GetMe`, `Resolve` (returns chat +
  sanitized error code/desc). `client.Bot(token)` is the factory. Token never logged/returned.
- **REST** `internal/server/integrations/telegram_bot.go`: `GET/POST /bot`, `POST /bot/verify`,
  `DELETE /bot` — admin-only; save/verify rate-limited (`AuthRateLimit`). Destination connect
  returns `bot_token_missing` when no org bot is configured.
- **Config**: `TELEGRAM_ALLOW_GLOBAL_FALLBACK` (default false).

## Tests (shipped)
- store `bot_credentials_test.go`: **token stored as ciphertext** (not plaintext), decrypt
  round-trip, **token absent from the DTO JSON**, `token_last4`, **org isolation**, revoke wipes
  token, reconnect-replaces.
- control `bot_test.go`: SaveBotToken verifies via getMe; invalid token → `bot_token_invalid`;
  **channel connect requires the ORG bot** (missing → `bot_token_missing`, then succeeds after
  save); **@username / handle / t.me/handle / https://t.me/handle normalization**; revoke disables
  delivery. Existing connect/notify tests updated for the per-org/fallback model.

## Per-workspace webhook — PENDING (next)
Each per-org bot has its own webhook. Safe design (next PR): `POST /api/telegram/webhook/:botPublicId`
+ secret header; the handler resolves the bot credential by `botPublicId` + secret, then dispatches
in that org's context. Raw token never in the URL. Until then: **channel delivery works via
sendMessage with the stored org token** (no webhook); DM commands + private-channel channel_post
connect remain on the platform/dev bot.

## Confirmations
No source file >200 lines · token encrypted at rest + never returned/logged · chat_id never exposed
· errors sanitized + specific · no direct `/comment` execution · no Telegram→Extension path ·
Facebook hot path untouched · no Vision.

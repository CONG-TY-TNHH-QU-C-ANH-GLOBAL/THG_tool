-- Per-ORG Telegram bot credentials (spec: specs/TELEGRAM_PER_ORG_BOT.md). Multi-tenant SaaS fix:
-- each workspace connects its OWN bot (created in BotFather) and channel delivery uses THAT bot's
-- token — no single global TELEGRAM_BOT_TOKEN for tenants. The token is a customer secret: stored
-- ENCRYPTED at rest (auth.Encrypt / AES-256-GCM with the app ENCRYPTION_KEY), never returned to the
-- UI, never logged. Only safe fields (bot_username, display name, last4, status) are exposed.
CREATE TABLE telegram_bot_credentials (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id             INTEGER  NOT NULL UNIQUE,        -- one bot per workspace (workspace == org)
    bot_id             INTEGER  NOT NULL DEFAULT 0,     -- Telegram bot user id (from getMe)
    bot_username       TEXT     NOT NULL DEFAULT '',    -- @your_bot
    bot_display_name   TEXT     NOT NULL DEFAULT '',
    token_encrypted    TEXT     NOT NULL DEFAULT '',    -- AES-256-GCM ciphertext; NEVER plaintext
    token_last4        TEXT     NOT NULL DEFAULT '',    -- safe display only
    status             TEXT     NOT NULL DEFAULT 'active', -- active | invalid | revoked | needs_attention
    created_by_user_id INTEGER  NOT NULL DEFAULT 0,
    last_verified_at   DATETIME,
    last_error         TEXT     NOT NULL DEFAULT '',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at         DATETIME
);

-- SaaS Platform plane — conversation threads, in-app notifications, Telegram
-- control plane (PostgreSQL platform baseline, database boundary sprint PR4).
-- Translated from the frozen SQLite baseline with __sqlite migrations folded
-- in (0013-0016 telegram tables + chat_id, 0018 notifications). Bot tokens
-- stay encrypted at rest (token_encrypted, never plaintext, never logged).

CREATE TABLE IF NOT EXISTS conversation_threads (
    id               BIGSERIAL PRIMARY KEY,
    lead_id          BIGINT DEFAULT 0,
    platform         TEXT NOT NULL DEFAULT 'facebook',
    profile_url      TEXT NOT NULL,
    profile_name     TEXT DEFAULT '',
    niche            TEXT DEFAULT 'logistics',
    status           TEXT NOT NULL DEFAULT 'initiated',
    last_outbound_at TIMESTAMPTZ,
    last_inbound_at  TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    unread_count     INTEGER NOT NULL DEFAULT 0,
    org_id           BIGINT NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_thread_org_profile ON conversation_threads(org_id, profile_url);
CREATE INDEX IF NOT EXISTS idx_thread_status ON conversation_threads(status);

CREATE TABLE IF NOT EXISTS conversation_messages (
    id           BIGSERIAL PRIMARY KEY,
    thread_id    BIGINT NOT NULL,
    direction    TEXT NOT NULL,
    content      TEXT NOT NULL,
    ai_generated INTEGER DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (thread_id) REFERENCES conversation_threads(id)
);
CREATE INDEX IF NOT EXISTS idx_conv_msg_thread ON conversation_messages(thread_id, created_at);

CREATE TABLE IF NOT EXISTS notifications (
    id           BIGSERIAL PRIMARY KEY,
    org_id       BIGINT NOT NULL,
    user_id      BIGINT NOT NULL DEFAULT 0,
    type         TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    read_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, read_at, created_at);
CREATE INDEX IF NOT EXISTS idx_notifications_org ON notifications(org_id, user_id, read_at, created_at);

CREATE TABLE IF NOT EXISTS telegram_settings (
    id               BIGSERIAL PRIMARY KEY,
    org_id           BIGINT NOT NULL UNIQUE,
    enabled          INTEGER NOT NULL DEFAULT 0,
    bot_username     TEXT NOT NULL DEFAULT '',
    webhook_last_at  TIMESTAMPTZ,
    webhook_last_err TEXT NOT NULL DEFAULT '',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS telegram_bind_codes (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL,
    user_id    BIGINT NOT NULL,
    code       TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used       INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tg_bind_codes_org ON telegram_bind_codes(org_id, code);

CREATE TABLE IF NOT EXISTS telegram_bindings (
    id                BIGSERIAL PRIMARY KEY,
    org_id            BIGINT NOT NULL,
    user_id           BIGINT NOT NULL,
    telegram_user_id  BIGINT NOT NULL,
    telegram_username TEXT NOT NULL DEFAULT '',
    display_name      TEXT NOT NULL DEFAULT '',
    role              TEXT NOT NULL DEFAULT '',
    alert_recipient   INTEGER NOT NULL DEFAULT 1,
    status            TEXT NOT NULL DEFAULT 'active',
    bound_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_command_at   TIMESTAMPTZ,
    revoked_at        TIMESTAMPTZ,
    chat_id           BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tg_bindings_org ON telegram_bindings(org_id, status);
CREATE INDEX IF NOT EXISTS idx_tg_bindings_org_user ON telegram_bindings(org_id, user_id);
CREATE INDEX IF NOT EXISTS idx_tg_bindings_org_tg ON telegram_bindings(org_id, telegram_user_id);

CREATE TABLE IF NOT EXISTS telegram_alert_prefs (
    id             BIGSERIAL PRIMARY KEY,
    org_id         BIGINT NOT NULL UNIQUE,
    alerts_enabled INTEGER NOT NULL DEFAULT 1,
    channel_filter TEXT NOT NULL DEFAULT 'all',
    alert_types    TEXT NOT NULL DEFAULT '[]',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS telegram_audit (
    id               BIGSERIAL PRIMARY KEY,
    org_id           BIGINT NOT NULL,
    user_id          BIGINT NOT NULL DEFAULT 0,
    telegram_user_id BIGINT NOT NULL DEFAULT 0,
    action           TEXT NOT NULL,
    result           TEXT NOT NULL DEFAULT '',
    metadata         TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tg_audit_org ON telegram_audit(org_id, created_at);

CREATE TABLE IF NOT EXISTS telegram_destinations (
    id                   BIGSERIAL PRIMARY KEY,
    org_id               BIGINT NOT NULL,
    destination_type     TEXT NOT NULL DEFAULT 'channel',
    chat_id              BIGINT NOT NULL,
    title                TEXT NOT NULL DEFAULT '',
    username             TEXT NOT NULL DEFAULT '',
    invite_link          TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'active',
    event_types          TEXT NOT NULL DEFAULT '[]',
    channel_filter       TEXT NOT NULL DEFAULT 'all',
    delivery_mode        TEXT NOT NULL DEFAULT 'immediate',
    connected_by_user_id BIGINT NOT NULL DEFAULT 0,
    last_delivery_at     TIMESTAMPTZ,
    last_error           TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at           TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_tg_dest_org ON telegram_destinations(org_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tg_dest_org_chat ON telegram_destinations(org_id, chat_id);

CREATE TABLE IF NOT EXISTS telegram_bot_credentials (
    id                 BIGSERIAL PRIMARY KEY,
    org_id             BIGINT NOT NULL UNIQUE,
    bot_id             BIGINT NOT NULL DEFAULT 0,
    bot_username       TEXT NOT NULL DEFAULT '',
    bot_display_name   TEXT NOT NULL DEFAULT '',
    token_encrypted    TEXT NOT NULL DEFAULT '',
    token_last4        TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL DEFAULT 'active',
    created_by_user_id BIGINT NOT NULL DEFAULT 0,
    last_verified_at   TIMESTAMPTZ,
    last_error         TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at         TIMESTAMPTZ
);

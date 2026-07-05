-- SaaS Platform plane — Telegram delivery: notification destinations +
-- per-org bot credentials (PostgreSQL platform baseline, database boundary
-- sprint PR4). Moved verbatim from 0105 (Sonar duplicate-literal split —
-- table definitions, defaults, indexes, and column order are unchanged).
-- Folds __sqlite 0015 + 0016. Bot tokens stay encrypted at rest
-- (token_encrypted, never plaintext, never logged).

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

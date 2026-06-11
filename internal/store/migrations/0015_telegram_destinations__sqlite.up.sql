-- Telegram NOTIFICATION DESTINATIONS (spec: specs/TELEGRAM_CHANNEL_DESTINATIONS.md). Product
-- pivot: Telegram is a workspace NOTIFICATION CHANNEL for automation operations; the bot is the
-- transport/sender. A destination is where operational events are delivered — primarily a Telegram
-- CHANNEL (also group / personal_dm). telegram_bindings (PR-1) is kept for optional personal-DM
-- recipients + command auth; destinations are the new PRIMARY delivery model.
CREATE TABLE telegram_destinations (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id               INTEGER  NOT NULL,
    destination_type     TEXT     NOT NULL DEFAULT 'channel', -- channel | group | personal_dm
    chat_id              INTEGER  NOT NULL,
    title                TEXT     NOT NULL DEFAULT '',
    username             TEXT     NOT NULL DEFAULT '',         -- public @handle, '' for private
    invite_link          TEXT     NOT NULL DEFAULT '',
    status               TEXT     NOT NULL DEFAULT 'active',   -- active | disabled | needs_attention
    event_types          TEXT     NOT NULL DEFAULT '[]',       -- JSON array of subscribed event keys
    channel_filter       TEXT     NOT NULL DEFAULT 'all',      -- all | facebook | taobao | 1688
    delivery_mode        TEXT     NOT NULL DEFAULT 'immediate',-- immediate | digest (future)
    connected_by_user_id INTEGER  NOT NULL DEFAULT 0,
    last_delivery_at     DATETIME,
    last_error           TEXT     NOT NULL DEFAULT '',
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at           DATETIME
);
CREATE INDEX IF NOT EXISTS idx_tg_dest_org ON telegram_destinations(org_id, status);
-- One destination per (org, chat) — reconnecting the same channel updates the existing row.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tg_dest_org_chat ON telegram_destinations(org_id, chat_id);

-- SaaS Platform plane — conversation threads + in-app notifications
-- (PostgreSQL platform baseline, database boundary sprint PR4). Translated
-- from the frozen SQLite baseline with __sqlite migrations folded in
-- (0018 notifications). The Telegram control plane lives in
-- 0109_platform_telegram_bindings / 0110_platform_telegram_delivery.

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

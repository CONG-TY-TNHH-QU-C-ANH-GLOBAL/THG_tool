-- SaaS Platform plane — Telegram control plane: settings, pairing, user
-- bindings, alert prefs, audit (PostgreSQL platform baseline, database
-- boundary sprint PR4). Moved verbatim from 0105 (Sonar duplicate-literal
-- split — table definitions, defaults, indexes, and column order are
-- unchanged). Folds __sqlite 0013 + 0014 (chat_id).

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

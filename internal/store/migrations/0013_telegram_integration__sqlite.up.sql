-- Telegram integration (Omnichannel/Telegram track, spec:
-- specs/OMNICHANNEL_SALES_COPILOT_TELEGRAM_TRACK.md + specs/TELEGRAM_INTEGRATION_UI.md).
-- Tenant-scoped control-plane: per-org enable flag + webhook health, one-time bind codes,
-- user<->telegram bindings, org-level alert preferences, and an append-only audit trail.
-- Channel-neutral: alert preferences carry a channel_filter so Facebook/Taobao/1688 share one
-- model. Action EXECUTION is NOT enabled here (gated by TELEGRAM_ACTIONS_ENABLED, default off).

-- Per-org integration settings + webhook health.
CREATE TABLE telegram_settings (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id            INTEGER NOT NULL UNIQUE,
    enabled           INTEGER  NOT NULL DEFAULT 0,
    bot_username      TEXT     NOT NULL DEFAULT '',
    webhook_last_at   DATETIME,
    webhook_last_err  TEXT     NOT NULL DEFAULT '',
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- One-time pairing codes: a user generates a code in the web app, then sends /bind <code> to
-- the bot. Short-lived; single-use.
CREATE TABLE telegram_bind_codes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id     INTEGER  NOT NULL,
    user_id    INTEGER  NOT NULL,
    code       TEXT     NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    used       INTEGER  NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_tg_bind_codes_org ON telegram_bind_codes(org_id, code);

-- user <-> telegram_user_id bindings. status = active | revoked (revocation is audited, never
-- a hard delete). alert_recipient flags whether this binding receives push notifications.
CREATE TABLE telegram_bindings (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id            INTEGER  NOT NULL,
    user_id           INTEGER  NOT NULL,
    telegram_user_id  INTEGER  NOT NULL,
    telegram_username TEXT     NOT NULL DEFAULT '',
    display_name      TEXT     NOT NULL DEFAULT '',
    role              TEXT     NOT NULL DEFAULT '',
    alert_recipient   INTEGER  NOT NULL DEFAULT 1,
    status            TEXT     NOT NULL DEFAULT 'active',
    bound_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_command_at   DATETIME,
    revoked_at        DATETIME
);
CREATE INDEX IF NOT EXISTS idx_tg_bindings_org ON telegram_bindings(org_id, status);
CREATE INDEX IF NOT EXISTS idx_tg_bindings_org_user ON telegram_bindings(org_id, user_id);
CREATE INDEX IF NOT EXISTS idx_tg_bindings_org_tg ON telegram_bindings(org_id, telegram_user_id);

-- Org-level alert preferences. channel_filter is channel-neutral (all|facebook|taobao|1688);
-- alert_types is a JSON array of enabled alert kinds.
CREATE TABLE telegram_alert_prefs (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id         INTEGER  NOT NULL UNIQUE,
    alerts_enabled INTEGER  NOT NULL DEFAULT 1,
    channel_filter TEXT     NOT NULL DEFAULT 'all',
    alert_types    TEXT     NOT NULL DEFAULT '[]',
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Append-only audit of every Telegram control-plane event (bind code issued, bind ok/fail,
-- revoke, test notification, pause/resume, unauthorized attempt, alert delivery).
CREATE TABLE telegram_audit (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id           INTEGER  NOT NULL,
    user_id          INTEGER  NOT NULL DEFAULT 0,
    telegram_user_id INTEGER  NOT NULL DEFAULT 0,
    action           TEXT     NOT NULL,
    result           TEXT     NOT NULL DEFAULT '',
    metadata         TEXT     NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_tg_audit_org ON telegram_audit(org_id, created_at);

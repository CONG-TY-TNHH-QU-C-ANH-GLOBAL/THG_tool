-- SaaS Platform plane — accounts identity truth + connector control plane
-- (PostgreSQL platform baseline, database boundary sprint PR4). Translated
-- from the frozen SQLite baseline with __sqlite ALTERs folded in
-- (0007/0008/0019 agent_tokens columns, 0017 accounts.assignment_paused,
-- 0021 active-profile unique index). The 0021 legacy-dedup UPDATE is NOT
-- ported: a fresh PG database has no legacy duplicate rows; the unique
-- index enforces the invariant structurally.

CREATE TABLE IF NOT EXISTS accounts (
    id                BIGSERIAL PRIMARY KEY,
    platform          TEXT NOT NULL DEFAULT 'facebook',
    name              TEXT NOT NULL,
    email             TEXT DEFAULT '',
    cookies_json      TEXT DEFAULT '',
    proxy_url         TEXT DEFAULT '',
    user_agent        TEXT DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'active',
    notes             TEXT DEFAULT '',
    last_used         TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    org_id            BIGINT NOT NULL DEFAULT 1,
    assigned_user_id  BIGINT DEFAULT 0,
    browser_logged_in INTEGER NOT NULL DEFAULT 0,
    fb_user_id        TEXT NOT NULL DEFAULT '',
    fb_display_name   TEXT NOT NULL DEFAULT '',
    fb_username       TEXT NOT NULL DEFAULT '',
    fb_profile_url    TEXT NOT NULL DEFAULT '',
    assignment_paused INTEGER NOT NULL DEFAULT 0,
    checkpoint_count  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_accounts_platform ON accounts(platform, status);
CREATE UNIQUE INDEX IF NOT EXISTS uq_accounts_org_fb_identity
    ON accounts(org_id, fb_user_id) WHERE fb_user_id <> '';

CREATE TABLE IF NOT EXISTS agent_tokens (
    id                         BIGSERIAL PRIMARY KEY,
    org_id                     BIGINT NOT NULL DEFAULT 0,
    name                       TEXT NOT NULL,
    token_hash                 TEXT NOT NULL UNIQUE,
    created_by                 BIGINT NOT NULL DEFAULT 0,
    hostname                   TEXT DEFAULT '',
    os                         TEXT DEFAULT '',
    version                    TEXT DEFAULT '',
    last_seen                  TIMESTAMPTZ,
    active                     INTEGER NOT NULL DEFAULT 1,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    kind                       TEXT NOT NULL DEFAULT 'worker',
    transport                  TEXT NOT NULL DEFAULT 'poll',
    assigned_account_id        BIGINT NOT NULL DEFAULT 0,
    capabilities_json          TEXT NOT NULL DEFAULT '{}',
    current_url                TEXT NOT NULL DEFAULT '',
    fb_user_id                 TEXT NOT NULL DEFAULT '',
    fb_display_name            TEXT NOT NULL DEFAULT '',
    fb_username                TEXT NOT NULL DEFAULT '',
    fb_profile_url             TEXT NOT NULL DEFAULT '',
    stream_status              TEXT NOT NULL DEFAULT 'idle',
    chrome_error               TEXT NOT NULL DEFAULT '',
    identity_confidence        TEXT NOT NULL DEFAULT '',
    identity_extraction_method TEXT NOT NULL DEFAULT '',
    identity_last_verified_at  TEXT NOT NULL DEFAULT '',
    browser_profile_id         TEXT NOT NULL DEFAULT '',
    machine_label              TEXT NOT NULL DEFAULT '',
    build_number               TEXT NOT NULL DEFAULT '',
    release_channel            TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_hash ON agent_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_kind ON agent_tokens(org_id, kind, active);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_org ON agent_tokens(org_id, active);
CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_tokens_active_profile
    ON agent_tokens(browser_profile_id)
    WHERE active = 1 AND browser_profile_id <> '' AND kind = 'extension_connector';

CREATE TABLE IF NOT EXISTS connector_commands (
    id           BIGSERIAL PRIMARY KEY,
    org_id       BIGINT NOT NULL,
    account_id   BIGINT NOT NULL,
    agent_id     BIGINT NOT NULL DEFAULT 0,
    type         TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL DEFAULT 'pending',
    error_msg    TEXT NOT NULL DEFAULT '',
    created_by   BIGINT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_connector_commands_account ON connector_commands(org_id, account_id, status, id);
CREATE INDEX IF NOT EXISTS idx_connector_commands_agent ON connector_commands(agent_id, status, id);

CREATE TABLE IF NOT EXISTS connector_pairing_codes (
    id                  BIGSERIAL PRIMARY KEY,
    org_id              BIGINT NOT NULL,
    code_hash           TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    created_by          BIGINT NOT NULL DEFAULT 0,
    assigned_account_id BIGINT NOT NULL DEFAULT 0,
    expires_at          TIMESTAMPTZ NOT NULL,
    used_at             TIMESTAMPTZ,
    device_token_id     BIGINT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_connector_pairing_hash ON connector_pairing_codes(code_hash);
CREATE INDEX IF NOT EXISTS idx_connector_pairing_org ON connector_pairing_codes(org_id, expires_at);

CREATE TABLE IF NOT EXISTS connector_screenshots (
    account_id      BIGINT NOT NULL,
    org_id          BIGINT NOT NULL,
    agent_id        BIGINT NOT NULL DEFAULT 0,
    image_data      TEXT NOT NULL,
    current_url     TEXT NOT NULL DEFAULT '',
    fb_user_id      TEXT NOT NULL DEFAULT '',
    fb_display_name TEXT NOT NULL DEFAULT '',
    fb_username     TEXT NOT NULL DEFAULT '',
    fb_profile_url  TEXT NOT NULL DEFAULT '',
    stream_status   TEXT NOT NULL DEFAULT '',
    chrome_error    TEXT NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (org_id, account_id)
);
CREATE INDEX IF NOT EXISTS idx_connector_screenshots_agent ON connector_screenshots(agent_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_connector_screenshots_org ON connector_screenshots(org_id, updated_at);

CREATE TABLE IF NOT EXISTS extension_policies (
    id                    BIGINT PRIMARY KEY CHECK (id = 1),
    latest_version        TEXT NOT NULL DEFAULT '',
    min_supported_version TEXT NOT NULL DEFAULT '',
    min_required_version  TEXT NOT NULL DEFAULT '',
    release_channel       TEXT NOT NULL DEFAULT 'stable',
    release_notes         TEXT NOT NULL DEFAULT '',
    update_url            TEXT NOT NULL DEFAULT '',
    update_instructions   TEXT NOT NULL DEFAULT '',
    force_update_after    TIMESTAMPTZ,
    updated_at            TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS selector_cache (
    id         BIGSERIAL PRIMARY KEY,
    action     TEXT NOT NULL,
    platform   TEXT NOT NULL,
    selectors  TEXT NOT NULL DEFAULT '{}',
    hit_count  INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version    INTEGER NOT NULL DEFAULT 1,
    fail_count INTEGER NOT NULL DEFAULT 0,
    deprecated INTEGER NOT NULL DEFAULT 0,
    dom_hash   TEXT NOT NULL DEFAULT '',
    UNIQUE(action, platform)
);

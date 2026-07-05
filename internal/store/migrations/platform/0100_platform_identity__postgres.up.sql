-- SaaS Platform plane — identity & tenancy (PostgreSQL platform baseline,
-- database boundary sprint PR4). Translated from the frozen SQLite baseline
-- (0001_legacy_baseline__sqlite) with later __sqlite ALTERs folded in
-- (0018 org_invites.revoked_at, 0020 staff_contact_profiles).
-- Type rules follow 0001_knowledge_os_baseline__postgres: BIGSERIAL/BIGINT
-- ids, TIMESTAMPTZ, INTEGER 0/1 flags (Go scans ints), TEXT payloads.
-- Owners per docs/architecture/DATABASE_OWNERSHIP.md.

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'sales',
    active        INTEGER NOT NULL DEFAULT 1,
    failed_logins INTEGER NOT NULL DEFAULT 0,
    locked_until  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    org_id        BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

CREATE TABLE IF NOT EXISTS organizations (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    domain       TEXT DEFAULT '',
    plan_tier    TEXT NOT NULL DEFAULT 'free',
    max_accounts INTEGER NOT NULL DEFAULT 1,
    active       INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    abbr         TEXT NOT NULL DEFAULT '',
    color        TEXT NOT NULL DEFAULT '#4f46e5',
    logo_path    TEXT NOT NULL DEFAULT '',
    avatar_path  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS org_invites (
    id            BIGSERIAL PRIMARY KEY,
    org_id        BIGINT NOT NULL,
    email         TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'sales',
    token         TEXT NOT NULL UNIQUE,
    created_by    BIGINT NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL,
    used_at       TIMESTAMPTZ,
    accepted_by   BIGINT NOT NULL DEFAULT 0,
    email_status  TEXT NOT NULL DEFAULT 'pending',
    email_sent_at TIMESTAMPTZ,
    email_error   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    revoked_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_org_invites_email ON org_invites(email, used_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_org_invites_org ON org_invites(org_id);
CREATE INDEX IF NOT EXISTS idx_org_invites_token ON org_invites(token);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS audit_logs (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT,
    action     TEXT NOT NULL,
    ip_address TEXT DEFAULT '',
    metadata   TEXT DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS user_execution_context (
    org_id             BIGINT NOT NULL,
    user_id            BIGINT NOT NULL,
    default_account_id BIGINT NOT NULL DEFAULT 0,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE IF NOT EXISTS user_context (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS staff_contact_profiles (
    user_id        BIGINT PRIMARY KEY,
    org_id         BIGINT NOT NULL,
    display_name   TEXT NOT NULL DEFAULT '',
    role_title     TEXT NOT NULL DEFAULT '',
    telegram       TEXT NOT NULL DEFAULT '',
    zalo           TEXT NOT NULL DEFAULT '',
    phone          TEXT NOT NULL DEFAULT '',
    email          TEXT NOT NULL DEFAULT '',
    preferred_cta  TEXT NOT NULL DEFAULT '',
    signature_text TEXT NOT NULL DEFAULT '',
    visibility     TEXT NOT NULL DEFAULT 'team',
    active         INTEGER NOT NULL DEFAULT 1,
    updated_at     TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_staff_contact_org ON staff_contact_profiles(org_id);

CREATE TABLE IF NOT EXISTS staff_kpi (
    user_id    BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    org_id     BIGINT NOT NULL DEFAULT 1,
    convs      INTEGER NOT NULL DEFAULT 0,
    converted  INTEGER NOT NULL DEFAULT 0,
    cmts       INTEGER NOT NULL DEFAULT 0,
    pts        INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_staff_kpi_org ON staff_kpi(org_id);

CREATE TABLE IF NOT EXISTS kpi_config (
    org_id     BIGINT PRIMARY KEY,
    conv_pts   INTEGER NOT NULL DEFAULT 10,
    conv2_pts  INTEGER NOT NULL DEFAULT 50,
    cmt_pts    INTEGER NOT NULL DEFAULT 2,
    bonus_pts  INTEGER NOT NULL DEFAULT 1000,
    bonus_amt  INTEGER NOT NULL DEFAULT 500000,
    pen_pts    INTEGER NOT NULL DEFAULT 300,
    pen_amt    INTEGER NOT NULL DEFAULT 100000,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed: default platform org (mirrors the SQLite baseline seed). Explicit id
-- insert does not advance a PG sequence — fix it so the next auto id is safe.
INSERT INTO organizations (id, name, domain, plan_tier, max_accounts)
    VALUES (1, 'THG Platform', 'thgfulfill.com', 'enterprise', 0)
    ON CONFLICT (id) DO NOTHING;
SELECT setval(pg_get_serial_sequence('organizations', 'id'),
    GREATEST(1, (SELECT MAX(id) FROM organizations)));

-- SaaS Platform plane — outbound coordination spine (PostgreSQL platform
-- baseline, database boundary sprint PR4). Translated from the frozen
-- SQLite baseline with __sqlite ALTERs folded in (0006 actor verification
-- on execution_attempts, 0023 action_ledger.event_type). Semantics are
-- BINDING: outbound_messages CAS/lease columns, append-only
-- execution_attempts + action_ledger (coordination-owned, insert-only).
-- Seed action_policies rows mirror the SQLite baseline.

CREATE TABLE IF NOT EXISTS outbound_messages (
    id                   BIGSERIAL PRIMARY KEY,
    org_id               BIGINT NOT NULL DEFAULT 0,
    type                 TEXT NOT NULL DEFAULT 'comment',
    platform             TEXT NOT NULL DEFAULT 'facebook',
    account_id           BIGINT NOT NULL DEFAULT 0,
    target_url           TEXT NOT NULL,
    target_name          TEXT DEFAULT '',
    content              TEXT NOT NULL,
    context              TEXT DEFAULT '',
    image_path           TEXT DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'draft',
    ai_model             TEXT DEFAULT '',
    sent_at              TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_by           TEXT NOT NULL DEFAULT '',
    claimed_at           TIMESTAMPTZ,
    execution_id         TEXT NOT NULL DEFAULT '',
    lease_expiry         TIMESTAMPTZ,
    execution_state      TEXT NOT NULL DEFAULT 'planned',
    verification_outcome TEXT,
    created_by           BIGINT NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_outbound_active_target
    ON outbound_messages(org_id, account_id, type, target_url)
    WHERE execution_state IN ('planned', 'executing');
CREATE INDEX IF NOT EXISTS idx_outbound_exec_lease
    ON outbound_messages(execution_state, lease_expiry) WHERE execution_state = 'executing';
CREATE INDEX IF NOT EXISTS idx_outbound_exec_state ON outbound_messages(org_id, execution_state);
CREATE INDEX IF NOT EXISTS idx_outbound_status ON outbound_messages(status);
CREATE INDEX IF NOT EXISTS idx_outbound_type_url_status ON outbound_messages(type, target_url, status);
CREATE INDEX IF NOT EXISTS idx_outbound_verify_outcome ON outbound_messages(org_id, verification_outcome);

-- APPEND-ONLY (coordination-owned; corrections are new rows, never UPDATE).
CREATE TABLE IF NOT EXISTS execution_attempts (
    id                  BIGSERIAL PRIMARY KEY,
    action_ledger_id    BIGINT NOT NULL DEFAULT 0,
    outbound_id         BIGINT NOT NULL DEFAULT 0,
    org_id              BIGINT NOT NULL,
    account_id          BIGINT NOT NULL DEFAULT 0,
    target_url          TEXT NOT NULL DEFAULT '',
    action_type         TEXT NOT NULL DEFAULT '',
    attempt             INTEGER NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'queued',
    outcome             TEXT NOT NULL DEFAULT '',
    failure_reason      TEXT NOT NULL DEFAULT '',
    evidence_json       TEXT NOT NULL DEFAULT '{}',
    dom_verified        INTEGER NOT NULL DEFAULT 0,
    network_verified    INTEGER NOT NULL DEFAULT 0,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at         TIMESTAMPTZ,
    transition_type     TEXT NOT NULL DEFAULT 'finalize',
    execution_id        TEXT NOT NULL DEFAULT '',
    resulting_state     TEXT NOT NULL DEFAULT '',
    resulting_outcome   TEXT,
    lease_expiry        TIMESTAMPTZ,
    created_by          BIGINT NOT NULL DEFAULT 0,
    expected_fb_user_id TEXT NOT NULL DEFAULT '',
    actual_fb_user_id   TEXT NOT NULL DEFAULT '',
    actor_verdict       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_execution_attempts_account
    ON execution_attempts(org_id, account_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_execution_attempts_latest
    ON execution_attempts(outbound_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_execution_attempts_ledger
    ON execution_attempts(action_ledger_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_execution_attempts_org_outcome
    ON execution_attempts(org_id, outcome, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_execution_attempts_outbound
    ON execution_attempts(outbound_id, attempt DESC);

-- APPEND-ONLY (coordination-owned; corrections emit new event rows).
CREATE TABLE IF NOT EXISTS action_ledger (
    id             BIGSERIAL PRIMARY KEY,
    org_id         BIGINT NOT NULL,
    action_type    TEXT NOT NULL,
    target_type    TEXT NOT NULL DEFAULT '',
    target_url     TEXT NOT NULL,
    account_id     BIGINT NOT NULL DEFAULT 0,
    outbound_id    BIGINT NOT NULL DEFAULT 0,
    performed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cooldown_until TIMESTAMPTZ,
    outcome        TEXT NOT NULL DEFAULT 'queued',
    reason         TEXT NOT NULL DEFAULT '',
    created_by     BIGINT NOT NULL DEFAULT 0,
    channel        TEXT NOT NULL DEFAULT 'facebook',
    event_type     TEXT NOT NULL DEFAULT 'action_attempted'
);
CREATE INDEX IF NOT EXISTS idx_action_ledger_account
    ON action_ledger(org_id, account_id, action_type, performed_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_ledger_engagement
    ON action_ledger(org_id, target_url, performed_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_ledger_member ON action_ledger(org_id, created_by, performed_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_ledger_target
    ON action_ledger(org_id, action_type, target_url, performed_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_ledger_event_outbound
    ON action_ledger(outbound_id, event_type, performed_at DESC)
    WHERE outbound_id > 0;

CREATE TABLE IF NOT EXISTS action_policies (
    id                 BIGSERIAL PRIMARY KEY,
    org_id             BIGINT NOT NULL,
    action_type        TEXT NOT NULL,
    dedup_scope        TEXT NOT NULL DEFAULT 'per_account',
    block_on_planned   INTEGER NOT NULL DEFAULT 0,
    block_on_executing INTEGER NOT NULL DEFAULT 1,
    cooldown_seconds   INTEGER NOT NULL DEFAULT 86400,
    conversation_aware INTEGER NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ,
    coordination_scope TEXT NOT NULL DEFAULT '',
    UNIQUE(org_id, action_type)
);

-- Seeds (mirror the SQLite baseline defaults). The comment/group_post/
-- profile_post rows take dedup_scope from the column DEFAULT
-- ('per_account'); only inbox overrides it — same seeded values as the
-- SQLite baseline, without repeating the default literal.
INSERT INTO action_policies (org_id, action_type, block_on_planned, block_on_executing, cooldown_seconds, conversation_aware) VALUES
    (0, 'comment',      1, 1, 86400, 0),
    (0, 'group_post',   1, 1, 86400, 0),
    (0, 'profile_post', 1, 1, 86400, 0)
    ON CONFLICT (org_id, action_type) DO NOTHING;
INSERT INTO action_policies (org_id, action_type, dedup_scope, block_on_planned, block_on_executing, cooldown_seconds, conversation_aware) VALUES
    (0, 'inbox', 'workspace', 1, 1, 86400, 1)
    ON CONFLICT (org_id, action_type) DO NOTHING;

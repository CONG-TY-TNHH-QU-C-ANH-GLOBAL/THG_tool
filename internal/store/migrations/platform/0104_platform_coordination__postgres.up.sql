-- SaaS Platform plane — coordination behaviour/runtime policy state, comment
-- verification pipeline, runtime audit stream, direct-post workflow state
-- (PostgreSQL platform baseline, database boundary sprint PR4). Translated
-- from the frozen SQLite baseline with __sqlite ALTERs folded in
-- (0006 actor block on account_runtime_state, 0010/0011 comment_reverify,
-- 0012 comment_verification_audit, 0022 direct_post_comment_workflows).

CREATE TABLE IF NOT EXISTS account_behaviour_profiles (
    account_id       BIGINT PRIMARY KEY,
    org_id           BIGINT NOT NULL DEFAULT 0,
    trust_level      TEXT NOT NULL DEFAULT 'warming',
    account_age_days INTEGER NOT NULL DEFAULT 0,
    persona_type     TEXT NOT NULL DEFAULT '',
    workspace_role   TEXT NOT NULL DEFAULT '',
    capabilities     TEXT NOT NULL DEFAULT '{}',
    caps_override    TEXT NOT NULL DEFAULT '{}',
    notes            TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_behaviour_profile_org
    ON account_behaviour_profiles(org_id, trust_level);

CREATE TABLE IF NOT EXISTS account_runtime_state (
    account_id             BIGINT PRIMARY KEY,
    org_id                 BIGINT NOT NULL DEFAULT 0,
    counters_day           TEXT NOT NULL DEFAULT '',
    comments_today         INTEGER NOT NULL DEFAULT 0,
    inbox_today            INTEGER NOT NULL DEFAULT 0,
    group_posts_today      INTEGER NOT NULL DEFAULT 0,
    profile_posts_today    INTEGER NOT NULL DEFAULT 0,
    risk_score             DOUBLE PRECISION NOT NULL DEFAULT 0,
    recent_failures        INTEGER NOT NULL DEFAULT 0,
    cooldown_until         TIMESTAMPTZ,
    last_action_at         TIMESTAMPTZ,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_blocked          INTEGER NOT NULL DEFAULT 0,
    actor_block_reason     TEXT NOT NULL DEFAULT '',
    actor_blocked_at       TIMESTAMPTZ,
    last_actor_verdict     TEXT NOT NULL DEFAULT '',
    last_actual_fb_user_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_runtime_state_org
    ON account_runtime_state(org_id, cooldown_until);

CREATE TABLE IF NOT EXISTS comment_reverify (
    id                  BIGSERIAL PRIMARY KEY,
    org_id              BIGINT NOT NULL,
    outbound_id         BIGINT NOT NULL,
    target_url          TEXT NOT NULL,
    account_id          BIGINT NOT NULL DEFAULT 0,
    created_by          BIGINT NOT NULL DEFAULT 0,
    content             TEXT NOT NULL DEFAULT '',
    scheduled_for       TIMESTAMPTZ NOT NULL,
    claimed_at          TIMESTAMPTZ,
    attempted_at        TIMESTAMPTZ,
    outcome             TEXT NOT NULL DEFAULT 'pending',
    reason              TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claim_count         INTEGER NOT NULL DEFAULT 0,
    claimed_by_token_id BIGINT NOT NULL DEFAULT 0,
    UNIQUE(outbound_id)
);
CREATE INDEX IF NOT EXISTS idx_comment_reverify_due ON comment_reverify(outcome, scheduled_for);

CREATE TABLE IF NOT EXISTS comment_verification_audit (
    id                    BIGSERIAL PRIMARY KEY,
    org_id                BIGINT NOT NULL,
    outbound_id           BIGINT NOT NULL,
    target_url            TEXT NOT NULL DEFAULT '',
    account_id            BIGINT NOT NULL DEFAULT 0,
    verified_by_user_id   BIGINT NOT NULL DEFAULT 0,
    source                TEXT NOT NULL DEFAULT '',
    previous_outcome      TEXT NOT NULL DEFAULT '',
    new_effective_outcome TEXT NOT NULL DEFAULT '',
    correction_ledger_id  BIGINT NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_comment_verify_audit_org
    ON comment_verification_audit(org_id, outbound_id);

CREATE TABLE IF NOT EXISTS runtime_events (
    id          BIGSERIAL PRIMARY KEY,
    org_id      BIGINT NOT NULL DEFAULT 0,
    account_id  BIGINT NOT NULL DEFAULT 0,
    event       TEXT NOT NULL,
    level       TEXT NOT NULL DEFAULT 'info',
    outbound_id BIGINT NOT NULL DEFAULT 0,
    attempt_id  BIGINT NOT NULL DEFAULT 0,
    target_url  TEXT NOT NULL DEFAULT '',
    attrs_json  TEXT NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_runtime_events_event_time
    ON runtime_events(event, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runtime_events_org_time
    ON runtime_events(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runtime_events_outbound
    ON runtime_events(outbound_id, created_at DESC)
    WHERE outbound_id > 0;

CREATE TABLE IF NOT EXISTS direct_post_comment_workflows (
    id                   BIGSERIAL PRIMARY KEY,
    org_id               BIGINT NOT NULL,
    requested_by_user_id BIGINT NOT NULL DEFAULT 0,
    user_role            TEXT NOT NULL DEFAULT '',
    account_id           BIGINT NOT NULL DEFAULT 0,
    canonical_post_url   TEXT NOT NULL,
    post_fbid            TEXT NOT NULL DEFAULT '',
    group_ref            TEXT NOT NULL DEFAULT '',
    prompt               TEXT NOT NULL DEFAULT '',
    lead_id              BIGINT,
    import_task_id       TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'import_queued',
    intake_key           TEXT NOT NULL,
    idempotency_key      TEXT NOT NULL,
    error_code           TEXT NOT NULL DEFAULT '',
    error_message        TEXT NOT NULL DEFAULT '',
    retry_count          INTEGER NOT NULL DEFAULT 0,
    lease_owner          TEXT NOT NULL DEFAULT '',
    lease_until          TIMESTAMPTZ,
    next_run_at          TIMESTAMPTZ,
    last_attempt_at      TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at         TIMESTAMPTZ,
    expires_at           TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_dpcw_idempotency_key
    ON direct_post_comment_workflows(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_dpcw_org_intake
    ON direct_post_comment_workflows(org_id, intake_key);
CREATE INDEX IF NOT EXISTS idx_dpcw_status_next_run
    ON direct_post_comment_workflows(status, next_run_at);
CREATE INDEX IF NOT EXISTS idx_dpcw_org_status
    ON direct_post_comment_workflows(org_id, status);

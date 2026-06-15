-- Direct-post intake → comment continuation: durable workflow state for the
-- P1 process manager (spec: specs/DIRECT_POST_INTAKE_WORKFLOW.md).
--
-- PR-1 = DATA FOUNDATION ONLY. This table is additive and feature-owned; NO
-- runtime code reads/writes it yet (only the typed coordination store + tests).
-- The PR-2 process manager (DB poller) drives it durably; there is no in-memory
-- callback or user_context KV continuation.
--
-- Two distinct idempotency concepts (deliberately separate):
--   * intake_key       = org_id + canonical_post_url  → one post imported ONCE;
--                        the imported lead is shared across requesters.
--   * idempotency_key  = org_id + canonical_post_url + acting account + user +
--                        action → one comment-workflow request per actor/action.
-- UNIQUE is on idempotency_key; intake_key is indexed (non-unique) so the poller/
-- intake service can find an in-flight import for the same post.
--
-- Rollback (repo is FORWARD-ONLY migrations; no .down.sql runner): manually
--   DROP TABLE direct_post_comment_workflows;
-- Safe BEFORE the feature is used. AFTER use, this drops pending workflow state
-- (in-flight imports/continuations are lost) — acceptable because the table is
-- additive and feature-owned, but it is NOT "no data loss". See the spec.

CREATE TABLE IF NOT EXISTS direct_post_comment_workflows (
    id                   INTEGER PRIMARY KEY,
    org_id               INTEGER   NOT NULL,
    requested_by_user_id INTEGER   NOT NULL DEFAULT 0,
    user_role            TEXT      NOT NULL DEFAULT '',
    account_id           INTEGER   NOT NULL DEFAULT 0,
    canonical_post_url   TEXT      NOT NULL,
    post_fbid            TEXT      NOT NULL DEFAULT '',
    group_ref            TEXT      NOT NULL DEFAULT '',
    prompt               TEXT      NOT NULL DEFAULT '',
    lead_id              INTEGER,
    import_task_id       TEXT      NOT NULL DEFAULT '',
    status               TEXT      NOT NULL DEFAULT 'import_queued',
    intake_key           TEXT      NOT NULL,
    idempotency_key      TEXT      NOT NULL,
    error_code           TEXT      NOT NULL DEFAULT '',
    error_message        TEXT      NOT NULL DEFAULT '',
    retry_count          INTEGER   NOT NULL DEFAULT 0,
    lease_owner          TEXT      NOT NULL DEFAULT '',
    lease_until          TIMESTAMP,
    next_run_at          TIMESTAMP,
    last_attempt_at      TIMESTAMP,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at         TIMESTAMP,
    expires_at           TIMESTAMP
);

-- One comment-workflow request per actor/action (the hard idempotency boundary).
CREATE UNIQUE INDEX IF NOT EXISTS uq_dpcw_idempotency_key
    ON direct_post_comment_workflows(idempotency_key);

-- Find an existing/in-flight import for the same post (share the imported lead).
CREATE INDEX IF NOT EXISTS idx_dpcw_org_intake
    ON direct_post_comment_workflows(org_id, intake_key);

-- Poller claim: due, runnable workflows ordered by next_run_at.
CREATE INDEX IF NOT EXISTS idx_dpcw_status_next_run
    ON direct_post_comment_workflows(status, next_run_at);

-- Per-org active-workflow scans.
CREATE INDEX IF NOT EXISTS idx_dpcw_org_status
    ON direct_post_comment_workflows(org_id, status);

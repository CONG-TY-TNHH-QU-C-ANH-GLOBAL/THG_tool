-- SaaS Platform plane — Facebook crawl: append-only run ledger (PR-M2B).
-- Runs are immutable execution attempts; a retry is a new row, never an
-- UPDATE of a terminal one. Runtime-inert; no consumer is wired yet.

CREATE TABLE IF NOT EXISTS facebook_crawl_runs (
    id               BIGSERIAL PRIMARY KEY,
    org_id           BIGINT NOT NULL,
    campaign_id      BIGINT NOT NULL,
    source_id        BIGINT NOT NULL,
    account_id       BIGINT,
    status           TEXT NOT NULL DEFAULT 'queued',
    exit_reason_code TEXT NOT NULL DEFAULT '',
    fresh_cutoff_at  TIMESTAMPTZ,
    attempt          INTEGER NOT NULL DEFAULT 1,
    retry_of_run_id  BIGINT,
    task_id          TEXT,

    posts_seen       INTEGER NOT NULL DEFAULT 0,
    fresh_lead_count INTEGER NOT NULL DEFAULT 0,
    stale_skipped    INTEGER NOT NULL DEFAULT 0,
    duplicate_count  INTEGER NOT NULL DEFAULT 0,
    unparsed_count   INTEGER NOT NULL DEFAULT 0,

    queued_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at       TIMESTAMPTZ,
    heartbeat_at     TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,

    CONSTRAINT uq_fb_crawl_runs_org_id_id
        UNIQUE (org_id, id),
    CONSTRAINT ck_fb_crawl_runs_status
        CHECK (
            status IN (
                'queued',
                'waiting_for_connector_upgrade',
                'running',
                'succeeded',
                'stopped_safe',
                'failed',
                'abandoned',
                'cancelled'
            )
        ),
    -- A running crawl must own an account; account safety depends on it.
    CONSTRAINT ck_fb_crawl_runs_running_requires_account
        CHECK (status <> 'running' OR account_id IS NOT NULL),
    CONSTRAINT ck_fb_crawl_runs_attempt
        CHECK (attempt > 0),
    CONSTRAINT ck_fb_crawl_runs_nonnegative_counters
        CHECK (
            posts_seen >= 0
            AND fresh_lead_count >= 0
            AND stale_skipped >= 0
            AND duplicate_count >= 0
            AND unparsed_count >= 0
        ),
    CONSTRAINT fk_fb_crawl_runs_campaign
        FOREIGN KEY (org_id, campaign_id)
        REFERENCES facebook_crawl_campaigns (org_id, id),
    CONSTRAINT fk_fb_crawl_runs_source
        FOREIGN KEY (org_id, campaign_id, source_id)
        REFERENCES facebook_crawl_campaign_sources
            (org_id, campaign_id, id),
    CONSTRAINT fk_fb_crawl_runs_account
        FOREIGN KEY (org_id, campaign_id, account_id)
        REFERENCES facebook_crawl_campaign_accounts
            (org_id, campaign_id, account_id),
    CONSTRAINT fk_fb_crawl_runs_retry_parent
        FOREIGN KEY (org_id, retry_of_run_id)
        REFERENCES facebook_crawl_runs (org_id, id)
);

-- Idempotency / concurrency invariants enforced at the database, not in app
-- code: one running run per account, one open run per source, one automatic
-- retry per parent, one run per ingest task.
CREATE UNIQUE INDEX IF NOT EXISTS ux_fb_crawl_runs_one_active_account
    ON facebook_crawl_runs (org_id, account_id)
    WHERE status = 'running' AND account_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_fb_crawl_runs_one_open_source
    ON facebook_crawl_runs (org_id, source_id)
    WHERE status IN (
        'queued',
        'waiting_for_connector_upgrade',
        'running'
    );

CREATE UNIQUE INDEX IF NOT EXISTS ux_fb_crawl_runs_one_retry_per_parent
    ON facebook_crawl_runs (org_id, retry_of_run_id)
    WHERE retry_of_run_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_fb_crawl_runs_org_task
    ON facebook_crawl_runs (org_id, task_id)
    WHERE task_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_fb_crawl_runs_org_source_created
    ON facebook_crawl_runs (org_id, source_id, queued_at DESC);

CREATE INDEX IF NOT EXISTS ix_fb_crawl_runs_org_account_status
    ON facebook_crawl_runs (org_id, account_id, status);

-- SaaS Platform plane — Facebook crawl: run ledger indexes (PR-M2B).
-- Runtime-inert. Depends on 0115 (facebook_crawl_runs).

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

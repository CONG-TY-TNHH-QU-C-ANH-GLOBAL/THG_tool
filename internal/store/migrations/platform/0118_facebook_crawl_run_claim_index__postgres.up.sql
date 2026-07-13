-- SaaS Platform plane — Facebook crawl: claim-queue read index (PR-M3B).
-- Runtime-inert. Depends on 0115 (facebook_crawl_runs). ClaimNextRun scans one
-- org's queued runs ordered by (queued_at, id); this partial index narrows the
-- scan to queued rows and supports that secondary ordering. It does not cover
-- the full ORDER BY — src.priority comes from the joined sources table — and it
-- does not change claim behavior. Normal transactional migration: the table is
-- new and empty, so no CONCURRENTLY build is needed.
CREATE INDEX IF NOT EXISTS ix_fb_crawl_runs_claim_queue
    ON facebook_crawl_runs (org_id, queued_at, id)
    WHERE status = 'queued';

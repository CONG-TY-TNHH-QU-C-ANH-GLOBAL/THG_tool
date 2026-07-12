-- SaaS Platform plane — Facebook crawl: fresh-lead identity index (PR-M2B).
-- Reserves canonical post identity before a lead is committed so concurrent or
-- repeated crawls cannot produce duplicate leads. Runtime-inert. Depends on
-- 0114 (facebook_crawl_runs).

CREATE TABLE IF NOT EXISTS facebook_crawl_lead_index (
    org_id          BIGINT NOT NULL,
    platform        TEXT NOT NULL DEFAULT 'facebook',
    post_dedup_hash TEXT NOT NULL,
    run_id          BIGINT NOT NULL,
    -- Nullable during claim-first processing: the runtime reserves the identity,
    -- creates the lead, then attaches lead_id, all in one transaction. A rolled
    -- back claim leaves no orphan reservation.
    lead_id         BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_fb_crawl_lead_index
        PRIMARY KEY (org_id, platform, post_dedup_hash),
    CONSTRAINT fk_fb_crawl_lead_index_run
        FOREIGN KEY (org_id, run_id)
        REFERENCES facebook_crawl_runs (org_id, id)
);

-- Canonical lead provenance FK (fk_fb_crawl_lead_index_lead -> leads(org_id, id))
-- is intentionally deferred: leads has no (org_id, id) unique anchor yet, and
-- adding one modifies the canonical lead table, which requires separate review.
-- It lands with the lead-write runtime consumer. lead_id stays an unconstrained
-- reservation column until then.

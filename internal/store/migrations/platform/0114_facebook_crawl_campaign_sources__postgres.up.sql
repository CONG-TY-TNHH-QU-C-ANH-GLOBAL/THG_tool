-- SaaS Platform plane — Facebook crawl: campaign account pool + sources
-- (PR-M2B). Runtime-inert. Depends on 0112 (accounts (org_id, id) anchor) and
-- 0113 (facebook_crawl_campaigns).

-- Which canonical Facebook accounts may serve a campaign. Composite FKs pin
-- both sides to the same org_id, so a cross-org account can never join a pool.
CREATE TABLE IF NOT EXISTS facebook_crawl_campaign_accounts (
    org_id      BIGINT NOT NULL,
    campaign_id BIGINT NOT NULL,
    account_id  BIGINT NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_fb_crawl_campaign_accounts
        PRIMARY KEY (org_id, campaign_id, account_id),
    CONSTRAINT fk_fb_crawl_campaign_accounts_campaign
        FOREIGN KEY (org_id, campaign_id)
        REFERENCES facebook_crawl_campaigns (org_id, id),
    CONSTRAINT fk_fb_crawl_campaign_accounts_account
        FOREIGN KEY (org_id, account_id)
        REFERENCES accounts (org_id, id)
);

CREATE TABLE IF NOT EXISTS facebook_crawl_campaign_sources (
    id                    BIGSERIAL PRIMARY KEY,
    org_id                BIGINT NOT NULL,
    campaign_id           BIGINT NOT NULL,
    source_url            TEXT NOT NULL,
    normalized_source_key TEXT NOT NULL,
    source_label          TEXT NOT NULL DEFAULT '',
    priority              INTEGER NOT NULL DEFAULT 0,
    preferred_account_id  BIGINT,
    cursor_last_post_at   TIMESTAMPTZ,
    last_run_at           TIMESTAMPTZ,
    status                TEXT NOT NULL DEFAULT 'active',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_fb_crawl_sources_org_campaign_id
        UNIQUE (org_id, campaign_id, id),
    CONSTRAINT uq_fb_crawl_sources_org_campaign_key
        UNIQUE (org_id, campaign_id, normalized_source_key),
    CONSTRAINT ck_fb_crawl_sources_status
        CHECK (status IN ('active', 'paused', 'archived')),
    CONSTRAINT fk_fb_crawl_sources_campaign
        FOREIGN KEY (org_id, campaign_id)
        REFERENCES facebook_crawl_campaigns (org_id, id),
    -- Nullable composite FK (MATCH SIMPLE): a null preferred_account_id means
    -- no sticky affinity and is not checked. No ON DELETE SET NULL — dropping a
    -- pool account with live source affinity must fail so the ownership
    -- transition is handled explicitly, not silently nulled.
    CONSTRAINT fk_fb_crawl_sources_preferred_account
        FOREIGN KEY (org_id, campaign_id, preferred_account_id)
        REFERENCES facebook_crawl_campaign_accounts
            (org_id, campaign_id, account_id)
);
CREATE INDEX IF NOT EXISTS ix_fb_crawl_sources_org_campaign_status
    ON facebook_crawl_campaign_sources (org_id, campaign_id, status);

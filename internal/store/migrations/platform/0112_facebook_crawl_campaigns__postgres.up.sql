-- SaaS Platform plane — Facebook multi-group fresh-lead crawl: campaigns
-- (PR-M2B). Runtime-inert additive schema; no consumer is wired yet.
-- See specs/facebook/FACEBOOK_CRAWL_CAMPAIGN_POSTGRES_SCHEMA_IMPLEMENTATION.md.

-- Composite tenant anchor: the campaign account pool's (org_id, account_id) FK
-- needs a non-partial unique key on accounts(org_id, id); accounts has only a
-- single-column PK today. Deterministic (no IF NOT EXISTS) so a same-named
-- index collision fails the migration visibly instead of masking drift.
-- Non-empty-table production apply is gated — see the accounts-anchor apply gate
-- in specs/facebook/FACEBOOK_CRAWL_CAMPAIGN_POSTGRES_SCHEMA_IMPLEMENTATION.md §4.
CREATE UNIQUE INDEX uq_accounts_org_id_id
    ON accounts (org_id, id);

CREATE TABLE IF NOT EXISTS facebook_crawl_campaigns (
    id                       BIGSERIAL PRIMARY KEY,
    org_id                   BIGINT NOT NULL,
    name                     TEXT NOT NULL,
    status                   TEXT NOT NULL DEFAULT 'active',
    freshness_window_minutes INTEGER NOT NULL DEFAULT 1440,
    cadence_minutes          INTEGER NOT NULL DEFAULT 240,
    max_items_per_run        INTEGER NOT NULL DEFAULT 50,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_fb_crawl_campaigns_org_id_id
        UNIQUE (org_id, id),
    CONSTRAINT ck_fb_crawl_campaigns_status
        CHECK (status IN ('active', 'paused', 'archived')),
    CONSTRAINT ck_fb_crawl_campaigns_freshness_window
        CHECK (freshness_window_minutes > 0),
    CONSTRAINT ck_fb_crawl_campaigns_cadence
        CHECK (cadence_minutes > 0),
    CONSTRAINT ck_fb_crawl_campaigns_max_items
        CHECK (max_items_per_run > 0)
);
CREATE INDEX IF NOT EXISTS ix_fb_crawl_campaigns_org_status
    ON facebook_crawl_campaigns (org_id, status);

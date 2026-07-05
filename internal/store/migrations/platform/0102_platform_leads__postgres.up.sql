-- SaaS Platform plane — leads domain (PostgreSQL platform baseline,
-- database boundary sprint PR4). Translated from the frozen SQLite
-- baseline with __sqlite ALTERs folded in (0009 leads archive columns).
-- Crawl artifacts live in 0108_platform_crawl__postgres.up.sql.

CREATE TABLE IF NOT EXISTS leads (
    id             BIGSERIAL PRIMARY KEY,
    org_id         BIGINT NOT NULL DEFAULT 0,
    source_type    TEXT NOT NULL,
    source_id      BIGINT NOT NULL,
    source_url     TEXT DEFAULT '',
    secondary_url  TEXT DEFAULT '',
    post_fbid      TEXT DEFAULT '',
    comment_fbid   TEXT DEFAULT '',
    group_fbid     TEXT DEFAULT '',
    platform       TEXT NOT NULL,
    author         TEXT,
    author_url     TEXT,
    content        TEXT NOT NULL,
    score          TEXT NOT NULL DEFAULT 'cold',
    service_match  TEXT DEFAULT 'None',
    author_role    TEXT DEFAULT 'unknown',
    pain_point     TEXT,
    ai_reasoning   TEXT,
    niche          TEXT NOT NULL DEFAULT 'logistics',
    classified_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    thread_role    TEXT NOT NULL DEFAULT 'intent_originator',
    archived_at    TIMESTAMPTZ,
    archive_reason TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_leads_dedup ON leads(source_type, source_id) WHERE source_id > 0;
CREATE INDEX IF NOT EXISTS idx_leads_org_score ON leads(org_id, score);
CREATE INDEX IF NOT EXISTS idx_leads_org_thread_role ON leads(org_id, thread_role);
CREATE INDEX IF NOT EXISTS idx_leads_score ON leads(score);
CREATE INDEX IF NOT EXISTS idx_leads_source_url ON leads(source_url) WHERE source_url <> '';
CREATE INDEX IF NOT EXISTS idx_leads_org_archived ON leads(org_id, archived_at);

CREATE TABLE IF NOT EXISTS classification_log (
    id              BIGSERIAL PRIMARY KEY,
    org_id          BIGINT NOT NULL,
    task_id         TEXT NOT NULL DEFAULT '',
    account_id      BIGINT NOT NULL DEFAULT 0,
    source_url      TEXT NOT NULL DEFAULT '',
    author_name     TEXT NOT NULL DEFAULT '',
    content_snippet TEXT NOT NULL DEFAULT '',
    ai_intent       TEXT NOT NULL DEFAULT '',
    ai_priority     TEXT NOT NULL DEFAULT '',
    ai_reason       TEXT NOT NULL DEFAULT '',
    ai_score        DOUBLE PRECISION NOT NULL DEFAULT 0,
    target_role     TEXT NOT NULL DEFAULT '',
    decision        TEXT NOT NULL,
    user_prompt     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_classification_log_org_decision ON classification_log(org_id, decision, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_classification_log_org_task ON classification_log(org_id, task_id, created_at DESC);

CREATE TABLE IF NOT EXISTS niches (
    id         BIGSERIAL PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    emoji      TEXT DEFAULT '🎯',
    active     INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seeds (mirror the SQLite baseline).
INSERT INTO niches (slug, name, emoji) VALUES ('logistics', 'Logistics & Vận chuyển', '🚛')
    ON CONFLICT (slug) DO NOTHING;
INSERT INTO niches (slug, name, emoji) VALUES ('tuyen_dung', 'Tuyển dụng', '👔')
    ON CONFLICT (slug) DO NOTHING;

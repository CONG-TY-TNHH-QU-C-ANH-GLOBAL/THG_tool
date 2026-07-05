-- SaaS Platform plane — crawl artifacts + server-side crawl jobs
-- (PostgreSQL platform baseline, database boundary sprint PR4). Translated
-- from the frozen SQLite baseline. scheduler_jobs (internal/jobs) is local
-- runtime and deliberately EXCLUDED; `jobs` here is the server-side crawl
-- job queue from the baseline. Split from the leads file to respect the
-- 200-line file-size guard — one domain per file.

CREATE TABLE IF NOT EXISTS groups (
    id         BIGSERIAL PRIMARY KEY,
    platform   TEXT NOT NULL,
    name       TEXT NOT NULL,
    url        TEXT NOT NULL UNIQUE,
    active     INTEGER NOT NULL DEFAULT 1,
    join_state TEXT NOT NULL DEFAULT 'none',
    last_scan  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    org_id     BIGINT NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_groups_active ON groups(active, platform);

CREATE TABLE IF NOT EXISTS group_quality (
    group_id              BIGINT PRIMARY KEY,
    category              TEXT DEFAULT '',
    relevance_score       DOUBLE PRECISION DEFAULT 0,
    professionalism_score DOUBLE PRECISION DEFAULT 0,
    content_quality_score DOUBLE PRECISION DEFAULT 0,
    spam_penalty          DOUBLE PRECISION DEFAULT 0,
    final_score           DOUBLE PRECISION DEFAULT 0,
    decision              TEXT DEFAULT 'monitor',
    reason                TEXT DEFAULT '',
    whitelist             INTEGER DEFAULT 0,
    blacklist             INTEGER DEFAULT 0,
    scored_at             TIMESTAMPTZ,
    last_post_at          TIMESTAMPTZ,
    weekly_post_count     INTEGER DEFAULT 0,
    candidate_yield       INTEGER DEFAULT 0,
    spam_yield            INTEGER DEFAULT 0,
    FOREIGN KEY (group_id) REFERENCES groups(id)
);
CREATE INDEX IF NOT EXISTS idx_group_quality_decision ON group_quality(decision);
CREATE INDEX IF NOT EXISTS idx_group_quality_score ON group_quality(final_score DESC);

CREATE TABLE IF NOT EXISTS posts (
    id            BIGSERIAL PRIMARY KEY,
    platform      TEXT NOT NULL,
    group_id      BIGINT,
    group_name    TEXT,
    url           TEXT,
    author        TEXT,
    author_url    TEXT,
    author_avatar TEXT,
    content       TEXT NOT NULL,
    images        TEXT DEFAULT '[]',
    reactions     INTEGER DEFAULT 0,
    comments      INTEGER DEFAULT 0,
    posted_at     TIMESTAMPTZ,
    scraped_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dedup_hash    TEXT NOT NULL UNIQUE,
    FOREIGN KEY (group_id) REFERENCES groups(id)
);
CREATE INDEX IF NOT EXISTS idx_posts_dedup ON posts(dedup_hash);
CREATE INDEX IF NOT EXISTS idx_posts_platform ON posts(platform, scraped_at);

CREATE TABLE IF NOT EXISTS comments (
    id         BIGSERIAL PRIMARY KEY,
    post_id    BIGINT,
    platform   TEXT NOT NULL,
    author     TEXT,
    author_url TEXT,
    content    TEXT NOT NULL,
    posted_at  TIMESTAMPTZ,
    scraped_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dedup_hash TEXT NOT NULL UNIQUE,
    FOREIGN KEY (post_id) REFERENCES posts(id)
);
CREATE INDEX IF NOT EXISTS idx_comments_dedup ON comments(dedup_hash);

CREATE TABLE IF NOT EXISTS inbox_messages (
    id          BIGSERIAL PRIMARY KEY,
    platform    TEXT NOT NULL,
    sender      TEXT,
    sender_url  TEXT,
    content     TEXT NOT NULL,
    is_read     INTEGER NOT NULL DEFAULT 0,
    received_at TIMESTAMPTZ,
    scraped_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scan_logs (
    id          BIGSERIAL PRIMARY KEY,
    platform    TEXT NOT NULL,
    group_count INTEGER DEFAULT 0,
    post_count  INTEGER DEFAULT 0,
    lead_count  INTEGER DEFAULT 0,
    duration    INTEGER DEFAULT 0,
    errors      TEXT DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS jobs (
    id             BIGSERIAL PRIMARY KEY,
    type           TEXT NOT NULL,
    platform       TEXT NOT NULL,
    target         TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    result         TEXT,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at     TIMESTAMPTZ,
    done_at        TIMESTAMPTZ,
    claimed_by     TEXT NOT NULL DEFAULT '',
    claimed_at     TIMESTAMPTZ,
    execution_mode TEXT NOT NULL DEFAULT 'server'
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);

CREATE TABLE IF NOT EXISTS org_crawl_intents (
    id                  BIGSERIAL PRIMARY KEY,
    org_id              BIGINT NOT NULL,
    account_id          BIGINT NOT NULL DEFAULT 0,
    name                TEXT NOT NULL DEFAULT '',
    prompt              TEXT NOT NULL DEFAULT '',
    intent              TEXT NOT NULL DEFAULT 'facebook_crawl',
    source_type         TEXT NOT NULL,
    source_url          TEXT NOT NULL,
    source_label        TEXT NOT NULL DEFAULT '',
    keywords_json       TEXT NOT NULL DEFAULT '[]',
    interval_minutes    INTEGER NOT NULL DEFAULT 30,
    max_items           INTEGER NOT NULL DEFAULT 50,
    enabled             INTEGER NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'active',
    dedup_hash          TEXT NOT NULL,
    cursor_last_post_id TEXT NOT NULL DEFAULT '',
    cursor_last_post_at TIMESTAMPTZ,
    cursor_updated_at   TIMESTAMPTZ,
    next_run_at         TIMESTAMPTZ NOT NULL,
    last_run_at         TIMESTAMPTZ,
    last_task_id        TEXT NOT NULL DEFAULT '',
    last_error          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, dedup_hash)
);
CREATE INDEX IF NOT EXISTS idx_org_crawl_intents_due ON org_crawl_intents(enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_org_crawl_intents_org ON org_crawl_intents(org_id, enabled);
CREATE INDEX IF NOT EXISTS idx_org_crawl_intents_status_due ON org_crawl_intents(status, next_run_at);

-- SaaS Platform plane — prompts/skills audit + server-side app workflow
-- state (PostgreSQL platform baseline, database boundary sprint PR4).
-- Translated from the frozen SQLite baseline with folds: prompt_logs
-- routing_decision_json (baseline) + user_id/index (0005 — its guarded
-- __postgres variant no-ops before this file creates the table);
-- task_leads.thread_role (layer-2 bootstrap ALTER). app_tasks/task_leads
-- are classified platform source-of-truth candidates (server workflow
-- state) per migrations/README.md; their SQLite bootstrap remains the MVP
-- path until the approved cutover sprint.

CREATE TABLE IF NOT EXISTS ai_memory (
    id           BIGSERIAL PRIMARY KEY,
    prompt_hash  TEXT NOT NULL UNIQUE,
    category     TEXT DEFAULT 'other',
    user_prompt  TEXT NOT NULL,
    best_action  TEXT DEFAULT '',
    best_args    TEXT DEFAULT '',
    use_count    INTEGER DEFAULT 1,
    success_rate DOUBLE PRECISION DEFAULT 1.0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_memory_hash ON ai_memory(prompt_hash);

CREATE TABLE IF NOT EXISTS prompt_logs (
    id                    BIGSERIAL PRIMARY KEY,
    org_id                BIGINT NOT NULL DEFAULT 0,
    account_id            BIGINT NOT NULL DEFAULT 0,
    source                TEXT NOT NULL DEFAULT 'telegram',
    user_prompt           TEXT NOT NULL,
    ai_response           TEXT DEFAULT '',
    action_taken          TEXT DEFAULT '',
    action_args           TEXT DEFAULT '',
    success               INTEGER NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    routing_decision_json TEXT NOT NULL DEFAULT '{}',
    user_id               BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_prompt_logs_org_created ON prompt_logs(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_logs_org_user ON prompt_logs(org_id, user_id, created_at);

CREATE TABLE IF NOT EXISTS org_skills (
    org_id     BIGINT NOT NULL,
    skill_id   TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    config     TEXT NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, skill_id)
);

CREATE TABLE IF NOT EXISTS skill_executions (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL,
    user_id    BIGINT NOT NULL DEFAULT 0,
    source     TEXT NOT NULL,
    skill_id   TEXT NOT NULL,
    args_json  TEXT NOT NULL DEFAULT '{}',
    summary    TEXT NOT NULL DEFAULT '',
    success    INTEGER NOT NULL DEFAULT 0,
    error      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_skill_executions_org ON skill_executions(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_skill_executions_skill ON skill_executions(skill_id, created_at DESC);

CREATE TABLE IF NOT EXISTS app_tasks (
    id             BIGSERIAL PRIMARY KEY,
    task_id        TEXT NOT NULL UNIQUE,
    org_id         BIGINT NOT NULL DEFAULT 0,
    intent         TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    total_fetched  INTEGER NOT NULL DEFAULT 0,
    total_returned INTEGER NOT NULL DEFAULT 0,
    error          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_app_tasks_org ON app_tasks(org_id, intent, status, created_at DESC);

CREATE TABLE IF NOT EXISTS task_leads (
    id                 BIGSERIAL PRIMARY KEY,
    task_id            TEXT NOT NULL,
    org_id             BIGINT NOT NULL DEFAULT 0,
    source_url         TEXT NOT NULL,
    author_profile_url TEXT NOT NULL DEFAULT '',
    author_name        TEXT NOT NULL DEFAULT '',
    content            TEXT NOT NULL DEFAULT '',
    lead_score         DOUBLE PRECISION NOT NULL DEFAULT 0,
    category           TEXT NOT NULL DEFAULT 'cold',
    signals_json       TEXT NOT NULL DEFAULT '[]',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    thread_role        TEXT NOT NULL DEFAULT 'intent_originator',
    UNIQUE(task_id, source_url)
);
CREATE INDEX IF NOT EXISTS idx_task_leads_org ON task_leads(org_id, category, lead_score DESC);

CREATE TABLE IF NOT EXISTS career_jobs (
    id            BIGSERIAL PRIMARY KEY,
    title         TEXT NOT NULL,
    description   TEXT DEFAULT '',
    location      TEXT DEFAULT '',
    requirements  TEXT DEFAULT '',
    benefits      TEXT DEFAULT '',
    email         TEXT DEFAULT '',
    url           TEXT DEFAULT '',
    is_active     INTEGER NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    salary        TEXT DEFAULT '',
    priority      TEXT DEFAULT 'medium',
    urgency_score INTEGER DEFAULT 50
);

CREATE TABLE IF NOT EXISTS price_items (
    id           BIGSERIAL PRIMARY KEY,
    service_name TEXT NOT NULL,
    price        TEXT NOT NULL,
    unit         TEXT DEFAULT '',
    notes        TEXT DEFAULT '',
    source       TEXT DEFAULT 'text',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

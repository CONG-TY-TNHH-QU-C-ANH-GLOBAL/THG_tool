-- SaaS Platform plane — knowledge SOURCE METADATA / upload registry
-- (PostgreSQL platform baseline, database boundary sprint PR4). Per the
-- data-plane doctrine clarification (DATABASE_OWNERSHIP.md): platform-owned
-- source metadata / upload records stay in the platform plane; the RAG
-- plane owns retrieval runtime data (chunks, embeddings, vector index —
-- NOT created here). knowledge_sources/assets/events/feedback already
-- exist on PG via 0001_knowledge_os_baseline__postgres + 0004; this file
-- adds only the remaining upload-registry tables from the SQLite baseline.

CREATE TABLE IF NOT EXISTS data_sources (
    id            BIGSERIAL PRIMARY KEY,
    org_id        BIGINT NOT NULL,
    type          TEXT NOT NULL,
    name          TEXT NOT NULL,
    source_url    TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending',
    item_count    INTEGER NOT NULL DEFAULT 0,
    summary       TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    last_error    TEXT NOT NULL DEFAULT '',
    last_sync_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_data_sources_org ON data_sources(org_id, type, status);

CREATE TABLE IF NOT EXISTS private_files (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    mime_type  TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_private_files_org ON private_files(org_id);

CREATE TABLE IF NOT EXISTS company_images (
    id               BIGSERIAL PRIMARY KEY,
    telegram_file_id TEXT NOT NULL,
    local_path       TEXT NOT NULL DEFAULT '',
    description      TEXT DEFAULT '',
    category         TEXT DEFAULT 'general',
    source_url       TEXT DEFAULT '',
    use_count        INTEGER DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_company_images_category ON company_images(category);

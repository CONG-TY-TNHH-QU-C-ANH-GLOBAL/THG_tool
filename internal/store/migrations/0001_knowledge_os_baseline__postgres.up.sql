-- Workspace Knowledge OS — PostgreSQL baseline.
--
-- This migration creates the Knowledge OS tables in PG-flavoured SQL.
-- It runs ONLY on a fresh Postgres instance: the migrator skips it
-- on existing SQLite installs (their baseline was recorded by the
-- legacy s.migrate() body as version 1).
--
-- Scope: knowledge_sources, knowledge_assets, knowledge_events ONLY.
-- The rest of the schema (accounts, leads, etc.) is SQLite-only at
-- this checkpoint and will land in subsequent per-domain migrations
-- as those teams need PG support. See specs/POSTGRES_COMPAT_PLAN.md §4.
--
-- Type choices, with rationale:
--   * BIGSERIAL for IDs   — 64-bit autoincrement; matches Go int64.
--                            (R3: PG INTEGER is 32-bit and would
--                            silently truncate large IDs.)
--   * TIMESTAMPTZ         — timezone-aware; PG returns UTC when the
--                            session is UTC. Round-trips cleanly with
--                            time.Time in Go.
--   * TEXT vs JSONB       — TEXT for opaque payloads (handled by Go);
--                            JSONB for columns we plan to query inside
--                            SQL (data_json supports future trace
--                            analytics).
--   * Partial UNIQUE INDEX with WHERE — R4: PG supports this AND lets
--                            ON CONFLICT match it when the WHERE
--                            clause aligns. Phrase identically to the
--                            SQLite version so the runtime ON CONFLICT
--                            statement keeps working.

CREATE TABLE IF NOT EXISTS knowledge_sources (
    id                     BIGSERIAL PRIMARY KEY,
    org_id                 BIGINT       NOT NULL,
    type                   TEXT         NOT NULL,
    label                  TEXT         NOT NULL,
    connection_config      TEXT         NOT NULL DEFAULT '{}',
    sync_policy            TEXT         NOT NULL DEFAULT 'manual',
    health_status          TEXT         NOT NULL DEFAULT 'healthy',
    health_message         TEXT         NOT NULL DEFAULT '',
    last_sync_at           TIMESTAMPTZ,
    last_sync_asset_count  INTEGER      NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_knowledge_sources_org
    ON knowledge_sources(org_id, health_status);
CREATE INDEX IF NOT EXISTS idx_knowledge_sources_sync
    ON knowledge_sources(sync_policy, last_sync_at);

CREATE TABLE IF NOT EXISTS knowledge_assets (
    id                    BIGSERIAL PRIMARY KEY,
    org_id                BIGINT       NOT NULL,
    source_id             BIGINT       NOT NULL,
    external_id           TEXT         NOT NULL DEFAULT '',
    type                  TEXT         NOT NULL,
    title                 TEXT         NOT NULL,
    description           TEXT         NOT NULL DEFAULT '',
    tags                  TEXT         NOT NULL DEFAULT '[]',
    payload               TEXT         NOT NULL DEFAULT '{}',
    state                 TEXT         NOT NULL DEFAULT 'pending',
    pinned                INTEGER      NOT NULL DEFAULT 0,
    boost                 INTEGER      NOT NULL DEFAULT 0,
    retrieval_count_30d   INTEGER      NOT NULL DEFAULT 0,
    conversion_count_30d  INTEGER      NOT NULL DEFAULT 0,
    last_retrieved_at     TIMESTAMPTZ,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    FOREIGN KEY (source_id) REFERENCES knowledge_sources(id)
);
CREATE INDEX IF NOT EXISTS idx_knowledge_assets_org_state
    ON knowledge_assets(org_id, state);
CREATE INDEX IF NOT EXISTS idx_knowledge_assets_org_source
    ON knowledge_assets(org_id, source_id);
-- Partial UNIQUE INDEX (idempotent-ingest guard, R4).
-- PG matches this in ON CONFLICT clauses provided the runtime SQL
-- contains the same WHERE — the knowledge_assets repository writes
-- exactly that statement (see knowledge_assets.go UpsertKnowledgeAsset).
CREATE UNIQUE INDEX IF NOT EXISTS uq_knowledge_assets_idem
    ON knowledge_assets(org_id, source_id, external_id)
    WHERE external_id <> '';
CREATE INDEX IF NOT EXISTS idx_knowledge_assets_org_pin_boost
    ON knowledge_assets(org_id, pinned DESC, boost DESC, retrieval_count_30d DESC);

CREATE TABLE IF NOT EXISTS knowledge_events (
    id            BIGSERIAL PRIMARY KEY,
    org_id        BIGINT       NOT NULL,
    event_type    TEXT         NOT NULL,
    retrieval_id  TEXT         NOT NULL DEFAULT '',
    source_type   TEXT         NOT NULL DEFAULT '',
    query         TEXT         NOT NULL DEFAULT '',
    data_json     TEXT         NOT NULL DEFAULT '{}',
    duration_ms   BIGINT       NOT NULL DEFAULT 0,
    occurred_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_knowledge_events_org_time
    ON knowledge_events(org_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_events_retrieval
    ON knowledge_events(org_id, retrieval_id);

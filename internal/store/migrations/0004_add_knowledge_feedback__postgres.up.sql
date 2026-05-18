-- Workspace Knowledge OS — Goal G10 (Human Feedback) substrate.
--
-- POSTGRES-ONLY migration: creates knowledge_feedback on a fresh PG
-- deployment. The same table is already present on SQLite installs
-- via the legacy s.migrate() body in schema.go, which is idempotent
-- (CREATE TABLE IF NOT EXISTS). Splitting along dialect lines keeps
-- the SQL native — BIGSERIAL for PG, AUTOINCREMENT for SQLite —
-- without forcing portability compromises.
--
-- Schema rules (per G10 invariants):
--   - APPEND-ONLY. No UPDATE column exposed by the repository;
--     no `revised_at` field; rows are immutable once written.
--   - org_id-first indexing — every analytics query is tenant-scoped.
--   - kind is the closed enum: thumbs_up | thumbs_down | approve |
--     reject | edit | rating. Validated at the Go boundary, not via
--     a SQL CHECK constraint (kept lenient so future kinds can ship
--     with a Go-side validate rather than a migration).
--   - data_json carries kind-specific payload; opaque to the store.
--
-- The retrieval engine MUST NOT read this table. That guarantee is
-- structural — only analytics handlers + offline enricher import
-- knowledge_feedback's read methods. See
-- internal/store/knowledge_feedback.go's _AutoTrainPolicyMarker.

CREATE TABLE IF NOT EXISTS knowledge_feedback (
    id            BIGSERIAL PRIMARY KEY,
    org_id        BIGINT       NOT NULL,
    user_id       BIGINT       NOT NULL DEFAULT 0,
    retrieval_id  TEXT         NOT NULL DEFAULT '',
    asset_id      BIGINT       NOT NULL DEFAULT 0,
    kind          TEXT         NOT NULL,
    data_json     TEXT         NOT NULL DEFAULT '{}',
    occurred_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_feedback_org_time
    ON knowledge_feedback(org_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_knowledge_feedback_retrieval
    ON knowledge_feedback(org_id, retrieval_id);

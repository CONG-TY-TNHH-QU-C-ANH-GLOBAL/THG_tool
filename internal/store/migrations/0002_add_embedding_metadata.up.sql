-- Knowledge OS — embedding metadata columns.
--
-- This migration adds the bookkeeping columns the embedding pipeline
-- needs to track which assets have been embedded, with which model,
-- and whether the embedding is still fresh relative to the asset's
-- text content. It runs on BOTH SQLite and Postgres.
--
-- Why metadata on both dialects when only PG stores vectors?
-- Because operators commonly start on SQLite and migrate to PG. If
-- the metadata columns exist on SQLite, the migration-day backfill
-- knows exactly which assets need embeddings (status='pending') and
-- which can be skipped. Without these columns, the cutover requires
-- regenerating 100% of the catalog. Cheap insurance.
--
-- Column responsibilities:
--   embedding_status      pending | generated | failed | skipped
--   embedding_model_version  stable model identifier (e.g.
--                            "openai:text-embedding-3-small:v1") —
--                            allows safe re-embed across model bumps
--   embedding_generated_at  UTC timestamp of last successful embed;
--                            NULL when never generated
--   embedding_input_hash    sha1 of (title || description || sortedTags);
--                            recomputed on every UpsertKnowledgeAsset.
--                            When new hash != stored hash, status
--                            flips back to 'pending' for re-embedding.
--   embedding_attempts      monotonic retry counter; >= MaxAttempts
--                            marks status='failed'
--   embedding_last_error    last-attempt error message for operator
--                            debugging via /api/org/knowledge/stats

ALTER TABLE knowledge_assets ADD COLUMN embedding_status        TEXT     NOT NULL DEFAULT 'pending';
ALTER TABLE knowledge_assets ADD COLUMN embedding_model_version TEXT     NOT NULL DEFAULT '';
ALTER TABLE knowledge_assets ADD COLUMN embedding_generated_at  TIMESTAMP;
ALTER TABLE knowledge_assets ADD COLUMN embedding_input_hash    TEXT     NOT NULL DEFAULT '';
ALTER TABLE knowledge_assets ADD COLUMN embedding_attempts      INTEGER  NOT NULL DEFAULT 0;
ALTER TABLE knowledge_assets ADD COLUMN embedding_last_error    TEXT     NOT NULL DEFAULT '';

-- Hot-path index: worker polls "WHERE org_id=? AND embedding_status='pending'
-- ORDER BY id LIMIT N". Composite (status, id) lets the planner skip
-- generated/failed rows without scanning. id-tail for deterministic order.
CREATE INDEX IF NOT EXISTS idx_knowledge_assets_embedding_pending
    ON knowledge_assets(embedding_status, id);

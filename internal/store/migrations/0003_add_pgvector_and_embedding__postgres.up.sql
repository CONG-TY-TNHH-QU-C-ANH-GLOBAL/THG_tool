-- Knowledge OS — pgvector extension + embedding column (PG only).
--
-- This migration runs ONLY on Postgres. It assumes:
--
--   1. The Postgres instance has pgvector installed at the OS level.
--      For RDS / Aurora: enable via parameter group.
--      For self-hosted: `apt install postgresql-15-pgvector` or
--      build from https://github.com/pgvector/pgvector.
--   2. The role running the migration has CREATE EXTENSION privilege.
--      On managed PG (RDS, Supabase), this requires the master role
--      OR pre-granted "rds_superuser"/"supabase_admin" membership.
--
-- If either condition fails, this migration errors at boot — that is
-- the correct failure mode (loud, blocking, deploy-time, not runtime).
--
-- Embedding dimension choice: 1536 matches OpenAI text-embedding-3-small.
-- text-embedding-3-large is 3072 dims; switching requires:
--   a) new migration adding a wider column, AND
--   b) backfill of every existing row (embeddings are not portable
--      across models even when dims happen to match).
-- The Embedder port validates Dimensions() against this column at
-- boot so an mismatched config refuses to start, not at first query.
--
-- Index choice: HNSW with vector_cosine_ops. Rationale:
--   - HNSW supports incremental inserts (no rebuild on every upsert).
--   - vector_cosine_ops matches the runtime similarity function
--     (cosine distance) we'll use in PR-2 (pgvector Searcher).
--   - m=16, ef_construction=64 are pgvector defaults — fine for
--     catalogs up to ~1M vectors. Larger workspaces should re-evaluate.
--   - IVFFlat is faster to build but requires periodic re-clustering
--     and degrades on incremental inserts. Not the right pick for our
--     "ingest streams" model.
--
-- The index is created NULL-safe: rows without embeddings (status !=
-- 'generated') are excluded via the partial index WHERE clause. This
-- keeps the index small while embeddings are being backfilled.

CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE knowledge_assets ADD COLUMN embedding VECTOR(1536);

CREATE INDEX IF NOT EXISTS idx_knowledge_assets_embedding_hnsw
    ON knowledge_assets
    USING hnsw (embedding vector_cosine_ops)
    WHERE embedding IS NOT NULL;

-- SaaS Platform plane — Reel Studio "enriched" format (PR-E1, per
-- docs/architecture/decisions/ADR-reel-studio-platform-module.md and the
-- Reel v2 plan). Extends the PR-R1 reel foundation (0111) for the new
-- data flow: an existing company-shot video is enriched with a HeyGen
-- avatar (corner) and Remotion-rendered translated subtitles, instead of
-- generating a clip from scratch.
--
-- Additive and own-table-only: only reel-owned tables are touched, so this
-- crosses no domain boundary. Postgres is the source of truth for reel
-- business state; media binaries live in object storage (R2) — these
-- columns carry object-storage KEYS/URLs, never blobs.
--
-- Money invariant (ADR §3): once a render is started, spend is committed.
-- render_idempotency_key is the reel-scoped guard against a retry/crash
-- charging a second render; it is NOT a reuse of the outbound spine's
-- CAS/lease mechanism (mixing those would be a boundary violation).

ALTER TABLE reels ADD COLUMN IF NOT EXISTS reel_type        TEXT NOT NULL DEFAULT 'enriched';
ALTER TABLE reels ADD COLUMN IF NOT EXISTS source_key       TEXT NOT NULL DEFAULT '';
ALTER TABLE reels ADD COLUMN IF NOT EXISTS input_branch     TEXT NOT NULL DEFAULT '';
ALTER TABLE reels ADD COLUMN IF NOT EXISTS avatar_key       TEXT NOT NULL DEFAULT '';
ALTER TABLE reels ADD COLUMN IF NOT EXISTS final_output_key TEXT NOT NULL DEFAULT '';
ALTER TABLE reels ADD COLUMN IF NOT EXISTS total_cost_usd   DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE reels ADD COLUMN IF NOT EXISTS render_idempotency_key TEXT;
ALTER TABLE reels ADD COLUMN IF NOT EXISTS render_lease_expiry    TIMESTAMPTZ;

-- One in-flight render per idempotency key. Partial index so the many rows
-- with a NULL key (drafts that never reached render) do not collide.
CREATE UNIQUE INDEX IF NOT EXISTS idx_reels_render_idem
    ON reels(render_idempotency_key) WHERE render_idempotency_key IS NOT NULL;

-- reel_transcripts: the understood content of the source video plus the
-- timing cues that drive subtitle placement. segments carries word/phrase
-- level [{text, from_ms, to_ms}] JSON so Remotion can sync captions to the
-- moment they are spoken — the single biggest quality risk if omitted.
-- org_id is carried directly (not join-only); the composite FK is the
-- tenant-isolation enforcement point, matching reel_scripts (0111).
CREATE TABLE IF NOT EXISTS reel_transcripts (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL DEFAULT 0,
    reel_id    BIGINT NOT NULL,
    segments   TEXT NOT NULL DEFAULT '[]', -- JSON: [{text, from_ms, to_ms}]
    lang_src   TEXT NOT NULL DEFAULT '',
    lang_tgt   TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT '',   -- 'whisper' | 'vision'
    cost_usd   DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (org_id, reel_id) REFERENCES reels(org_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_reel_transcripts_org_reel ON reel_transcripts(org_id, reel_id);

-- SaaS Platform plane — Reel Studio foundation (PR-R1, per
-- docs/architecture/decisions/ADR-reel-studio-platform-module.md). Postgres
-- is the source of truth for reel business state; SQLite carries no reel
-- schema (nothing in the current runtime reads/writes reels yet, so there is
-- no compatibility burden to preserve there).
--
-- v1 is intentionally narrow: reels + their script drafts only. Render
-- state (reel_shots / reel_render_jobs) is deferred to PR-R2, which is the
-- first PR that actually writes it (the VideoRenderer port).

CREATE TABLE IF NOT EXISTS reels (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL DEFAULT 0,
    title      TEXT NOT NULL DEFAULT '',
    brief      TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'draft',
    created_by BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Tenant-scoped alternate key so reel_scripts can carry a composite FK
    -- (org_id, reel_id) -> (org_id, id): Postgres requires a unique
    -- constraint on exactly the referenced column set.
    UNIQUE (org_id, id)
);
CREATE INDEX IF NOT EXISTS idx_reels_org_status ON reels(org_id, status);

-- reel_scripts: versioned dialogue/shot-list draft, one row per revision.
-- org_id is carried directly (not join-only) so org-scoped reads never need
-- to join through reels. The composite FK below is the tenant-isolation
-- enforcement point: it makes a cross-org (org_id, reel_id) pairing a
-- constraint violation at INSERT time, not just an application-level
-- convention — a caller cannot associate a script with a reel_id owned by
-- a different org even if it passes the wrong org_id by mistake or malice.
CREATE TABLE IF NOT EXISTS reel_scripts (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL DEFAULT 0,
    reel_id    BIGINT NOT NULL,
    version    INTEGER NOT NULL DEFAULT 1,
    content    TEXT NOT NULL DEFAULT '{}', -- JSON: dialogue/shot-list/caption
    approved   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (org_id, reel_id) REFERENCES reels(org_id, id) ON DELETE CASCADE,
    UNIQUE (org_id, reel_id, version)
);
CREATE INDEX IF NOT EXISTS idx_reel_scripts_org_reel ON reel_scripts(org_id, reel_id, version);

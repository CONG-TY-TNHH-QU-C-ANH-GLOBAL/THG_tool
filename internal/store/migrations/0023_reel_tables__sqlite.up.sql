-- Reel service: AI-scripted short-video generation that posts through the existing
-- outbound spine as a new action kind `post_reel` (spec: services/reel backend track).
--
-- Money invariant: once a render is started, spend is committed and cannot be cancelled.
-- The schema encodes that invariant — render_idempotency_key (UNIQUE) blocks double-charge,
-- render_lease_expiry detects orphaned jobs (→ render_stuck, human handles; never auto re-render),
-- and per-shot render_state mirrors the outbound CAS/lease pattern. There is deliberately NO
-- `cancelled` state reachable from `rendering`.
--
-- All three tables are org-scoped (tenant isolation, enforced by check_tenant_isolation.sh).
-- IDs are INTEGER AUTOINCREMENT to match every other table in this schema.
--
-- Rollback (repo is FORWARD-ONLY; no .down.sql runner): manually
--   DROP TABLE reel_shots; DROP TABLE reel_scripts; DROP TABLE reels;
-- and remove the post_reel action_policies row + the outbound_messages columns.
-- Safe before the feature is used; afterwards drops in-flight reel state.

-- reels aggregate: one row = one video task.
CREATE TABLE IF NOT EXISTS reels (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id                 INTEGER  NOT NULL,
    mission_id             TEXT     NOT NULL DEFAULT '',   -- optional link to a mission/campaign
    created_by             INTEGER  NOT NULL DEFAULT 0,    -- member user id, 0=system
    source                 TEXT     NOT NULL DEFAULT 'manual', -- 'manual' | 'auto'
    status                 TEXT     NOT NULL DEFAULT 'draft',
    -- input
    brief_style            TEXT     NOT NULL DEFAULT '',
    keywords               TEXT     NOT NULL DEFAULT '[]', -- JSON array
    product_refs           TEXT     NOT NULL DEFAULT '[]', -- JSON array of R2/upload keys
    target_duration_sec    INTEGER  NOT NULL DEFAULT 25,
    -- money: double-charge guard + orphan detection
    render_idempotency_key TEXT     UNIQUE,                -- set once when render starts
    render_lease_expiry    DATETIME,                       -- orphaned-job detector
    -- output
    final_output_key       TEXT     NOT NULL DEFAULT '',   -- assembled video key
    total_cost_usd         REAL     NOT NULL DEFAULT 0,    -- accrued shot cost (ROI)
    created_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_reels_org_status ON reels(org_id, status);

-- reel_scripts: versioned script + shot-list. One reel may iterate several versions before approve.
CREATE TABLE IF NOT EXISTS reel_scripts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    reel_id       INTEGER  NOT NULL REFERENCES reels(id),
    org_id        INTEGER  NOT NULL,
    version       INTEGER  NOT NULL DEFAULT 1,
    dialogue      TEXT     NOT NULL DEFAULT '',
    shot_list     TEXT     NOT NULL DEFAULT '[]',  -- JSON [{scene,kind,prompt,dur,voiceover}]
    caption       TEXT     NOT NULL DEFAULT '',    -- post caption (+ hashtags)
    verify_flags  TEXT     NOT NULL DEFAULT '[]',  -- JSON: facts requiring human verification
    approved      INTEGER  NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(reel_id, version)
);
CREATE INDEX IF NOT EXISTS idx_reel_scripts_reel ON reel_scripts(org_id, reel_id, version);

-- reel_shots: per-shot render state (CAS/lease pattern, mirrors outbound execution states).
CREATE TABLE IF NOT EXISTS reel_shots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    reel_id         INTEGER  NOT NULL REFERENCES reels(id),
    org_id          INTEGER  NOT NULL,
    scene           INTEGER  NOT NULL,                 -- assembly order
    kind            TEXT     NOT NULL DEFAULT 'broll',  -- broll|product|talking_head
    render_state    TEXT     NOT NULL DEFAULT 'planned',-- planned|rendering|done|failed|retry_scheduled
    provider        TEXT     NOT NULL DEFAULT '',       -- fake|seedance|heygen
    provider_job_id TEXT     NOT NULL DEFAULT '',       -- poll/webhook match key
    output_key      TEXT     NOT NULL DEFAULT '',       -- rendered clip key
    cost_usd        REAL     NOT NULL DEFAULT 0,
    attempts        INTEGER  NOT NULL DEFAULT 0,
    lease_expiry    DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(reel_id, scene)
);
CREATE INDEX IF NOT EXISTS idx_reel_shots_state ON reel_shots(render_state, lease_expiry);

-- Outbound spine extension: dedicated media columns so post_reel can carry a video
-- without overloading image_path (which is reserved for image attachments).
ALTER TABLE outbound_messages ADD COLUMN media_path TEXT NOT NULL DEFAULT '';
ALTER TABLE outbound_messages ADD COLUMN media_type TEXT NOT NULL DEFAULT ''; -- '' | 'image' | 'video'

-- Global default policy for the new action kind. Matches the post-style rows: no
-- block-on-planned (allow requeue), block-on-executing (safety), 24h cooldown, not
-- conversation-aware. Org overrides go via outbound.UpsertPolicy.
INSERT OR IGNORE INTO action_policies (org_id, action_type, dedup_scope, block_on_planned, block_on_executing, cooldown_seconds, conversation_aware) VALUES
  (0, 'post_reel', 'per_account', 0, 1, 86400, 0);

-- PR11 — PostgreSQL outbound lifecycle core (foundation, no runtime cutover).
--
-- This is the PostgreSQL home for the outbound task lifecycle the worker
-- drives: list planned -> claim -> finalize -> reset-stale. It is NOT run by
-- the in-house runtime migrator (which embeds only internal/store/migrations).
-- It is applied explicitly by the gated integration tests and, in future, by
-- an operator during the PostgreSQL cutover (see PR9 ADR + POSTGRES_COMPAT_PLAN).
-- SQLite remains the active runtime store.
--
-- Type choices (strict-typing parity with the SQLite columns in
-- internal/store/migrations/0001_legacy_baseline__sqlite.up.sql and the Go
-- scan targets in models.OutboundMessage):
--   * BIGSERIAL / BIGINT  — 64-bit ids; match Go int64 (PG INTEGER is 32-bit).
--   * TIMESTAMPTZ         — timezone-aware; round-trips with Go time.Time and
--                            sql.NullTime. SQLite stored these as DATETIME text;
--                            the PG adapter scans them as native time values.
--   * TEXT + CHECK        — execution_state / verification_outcome keep their
--                            EXACT existing string enum values (no PG ENUM type,
--                            per POSTGRES_COMPAT_PLAN — TEXT+CHECK matches code).
--   * No JSONB            — content / context / image_path / ai_model are opaque
--                            free-form text, not structured queryable JSON, so
--                            none become JSONB. Proof/evidence JSON lives in
--                            execution_attempts, which is intentionally out of
--                            this foundation's scope.
--
-- Deliberately OMITTED: the legacy `status` column. It is deprecated and
-- scheduled for removal in V2 Outbound PR2 (see DATABASE_OWNERSHIP.md); the
-- lifecycle methods never read or write it, so the forward PG schema does not
-- carry it.

CREATE TABLE IF NOT EXISTS outbound_messages (
    id                   BIGSERIAL    PRIMARY KEY,
    org_id               BIGINT       NOT NULL DEFAULT 0,
    type                 TEXT         NOT NULL DEFAULT 'comment',
    platform             TEXT         NOT NULL DEFAULT 'facebook',
    account_id           BIGINT       NOT NULL DEFAULT 0,
    target_url           TEXT         NOT NULL,
    target_name          TEXT         NOT NULL DEFAULT '',
    content              TEXT         NOT NULL,
    context              TEXT         NOT NULL DEFAULT '',
    image_path           TEXT         NOT NULL DEFAULT '',
    ai_model             TEXT         NOT NULL DEFAULT '',
    execution_state      TEXT         NOT NULL DEFAULT 'planned'
        CHECK (execution_state IN ('planned', 'executing', 'finished', 'expired')),
    verification_outcome TEXT
        CHECK (verification_outcome IS NULL OR verification_outcome IN (
            'verified_success', 'submitted_unverified', 'context_drift',
            'rate_limited', 'blocked', 'captcha', 'shadow_rejected',
            'execution_failed', 'target_not_reached')),
    claimed_by           TEXT         NOT NULL DEFAULT '',
    claimed_at           TIMESTAMPTZ,
    execution_id         TEXT         NOT NULL DEFAULT '',
    lease_expiry         TIMESTAMPTZ,
    sent_at              TIMESTAMPTZ,
    created_by           BIGINT       NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Serves the list-claimable read (org_id + execution_state, newest first).
CREATE INDEX IF NOT EXISTS idx_outbound_exec_state
    ON outbound_messages (org_id, execution_state, created_at DESC);

-- Serves reset-stale: only executing rows are ever scanned for lease eviction.
CREATE INDEX IF NOT EXISTS idx_outbound_exec_lease
    ON outbound_messages (execution_state, lease_expiry)
    WHERE execution_state = 'executing';

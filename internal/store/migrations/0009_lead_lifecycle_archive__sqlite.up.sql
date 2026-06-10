-- Lead Lifecycle + Work Queue + Auto Archive — PR-1 (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md).
-- Archiving is the ONE genuinely-new persistent lifecycle fact: freshness_state /
-- next_action / last_engaged_at are PROJECTED from the engagement ledger + threads and
-- are never stored (feedback_verified_state_centric). archived_at is an explicit,
-- reversible decision a human or the auto-archive job makes, so it lives on the
-- canonical `leads` table (ingest mirrors task_leads -> leads; dashboard reads `leads`).
--
-- No hard delete: archived leads stay in the table and in the engagement ledger, so
-- dedup + multi-actor coverage history still see them. They are merely hidden from the
-- default list and excluded from planner selection (GetLeadsFiltered AND archived_at IS NULL).
ALTER TABLE leads ADD COLUMN archived_at DATETIME;
ALTER TABLE leads ADD COLUMN archive_reason TEXT NOT NULL DEFAULT '';

-- Default list + planner both filter on (org_id, archived_at IS NULL); a partial-style
-- composite index keeps that scan cheap as the table grows.
CREATE INDEX IF NOT EXISTS idx_leads_org_archived ON leads(org_id, archived_at);

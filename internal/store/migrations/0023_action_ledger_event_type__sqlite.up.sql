-- Append-only ledger migration — PR1 ADDITIVE FOUNDATION ONLY
-- (specs/store/APPEND_ONLY_LEDGER_MIGRATION.md §2.1).
--
-- Adds the event_type discriminator column to action_ledger plus a supporting
-- partial index. ADDITIVE & default-safe: existing rows back-fill to
-- 'action_attempted', which is semantically what every legacy row already is —
-- an attempted action whose intent was 'queued'.
--
-- NO reader or writer path consumes event_type yet. This lays the foundation
-- for the later, SEPARATE reader/writer cutover PRs. No behavior change, no
-- ledger-semantics change, no append-only writer activation, no cutover.
ALTER TABLE action_ledger ADD COLUMN event_type TEXT NOT NULL DEFAULT 'action_attempted';

CREATE INDEX IF NOT EXISTS idx_action_ledger_event_outbound
	ON action_ledger(outbound_id, event_type, performed_at DESC)
	WHERE outbound_id > 0;

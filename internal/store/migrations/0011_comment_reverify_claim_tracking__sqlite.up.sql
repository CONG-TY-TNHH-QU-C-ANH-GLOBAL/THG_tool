-- Async Comment Reverify — claim tracking + self-heal (spec: specs/COMMENT_ASYNC_REVERIFY.md).
-- Diagnosis: a connector running STALE extension code claims jobs repeatedly (claimed_at
-- advances via the lease) but never reports (old code had a tab-access bug), so attempted_at
-- stayed NULL forever. Record WHO claimed (token id) + HOW MANY times, so the backend can
-- (a) attribute the claim to a specific agent_token + its extension version, and (b) self-
-- heal: retire a job claimed too many times without an attempt as error=claim_without_attempt
-- instead of looping forever.
ALTER TABLE comment_reverify ADD COLUMN claim_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE comment_reverify ADD COLUMN claimed_by_token_id INTEGER NOT NULL DEFAULT 0;

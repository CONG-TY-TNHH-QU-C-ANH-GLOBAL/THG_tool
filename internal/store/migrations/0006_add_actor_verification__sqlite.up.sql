-- P1b Verified Actor (specs/COMMENT_INTELLIGENCE_PIPELINE.md §7b).
--
-- Records, per execution, whether the Facebook identity that actually posted
-- (the live c_user the executor observed) matched the account's expected
-- identity (accounts.fb_user_id). A definite mismatch BLOCKS the account from
-- further auto-execute until an operator clears it — a real integrity gate,
-- not a UI label.
--
-- Append-only: the verdict is stamped on execution_attempts (the canonical
-- per-attempt audit row) and on the account's runtime state. The action_ledger
-- is NEVER mutated for it.
--
-- SQLite-only: execution_attempts / account_runtime_state are the legacy
-- execution tables and do not exist on the Postgres knowledge-OS baseline.

-- Per-attempt audit of the actor check (immutable once finalized).
ALTER TABLE execution_attempts ADD COLUMN expected_fb_user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE execution_attempts ADD COLUMN actual_fb_user_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE execution_attempts ADD COLUMN actor_verdict       TEXT NOT NULL DEFAULT '';

-- Per-account block + last-seen verdict. actor_blocked is the explicit gate
-- field CheckCapsTx denies on; it persists until an operator clears it (it is
-- NOT a timed cooldown).
ALTER TABLE account_runtime_state ADD COLUMN actor_blocked          INTEGER NOT NULL DEFAULT 0;
ALTER TABLE account_runtime_state ADD COLUMN actor_block_reason     TEXT    NOT NULL DEFAULT '';
ALTER TABLE account_runtime_state ADD COLUMN actor_blocked_at       DATETIME;
ALTER TABLE account_runtime_state ADD COLUMN last_actor_verdict     TEXT    NOT NULL DEFAULT '';
ALTER TABLE account_runtime_state ADD COLUMN last_actual_fb_user_id TEXT    NOT NULL DEFAULT '';

-- Async Comment Reverify (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-A). A submitted-but-
-- unverified comment (optimistic_success) often DID post in a FB group but the in-window
-- DOM proof missed it (lazy render / "Most relevant" sort). This table is the reverify
-- WORK QUEUE + audit: a few minutes later the extension re-opens the post, searches for the
-- comment by actor + normalized text, and reports back. On a positive match the backend
-- APPENDS a 'succeeded' correction to action_ledger (never mutates the old row); on a miss
-- it records not_found and the lead stays submitted_unverified until cooldown.
--
-- This is decoupled from outbound_messages/action_policies on purpose — reverify is a
-- read-only re-check, not a new outbound action, so it must not pass through the
-- dedup/cooldown/risk gates that govern real sends.
CREATE TABLE comment_reverify (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id        INTEGER NOT NULL,
    outbound_id   INTEGER NOT NULL,                 -- the original submitted_unverified outbound
    target_url    TEXT    NOT NULL,
    account_id    INTEGER NOT NULL DEFAULT 0,
    created_by    INTEGER NOT NULL DEFAULT 0,
    content       TEXT    NOT NULL DEFAULT '',       -- expected comment text (for the DOM search)
    scheduled_for DATETIME NOT NULL,                 -- when the row becomes claimable (>=2-5m after submit)
    claimed_at    DATETIME,
    attempted_at  DATETIME,
    outcome       TEXT    NOT NULL DEFAULT 'pending', -- pending | verified | not_found | error
    reason        TEXT    NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(outbound_id)                              -- one reverify per outbound (idempotent scheduling)
);

CREATE INDEX IF NOT EXISTS idx_comment_reverify_due ON comment_reverify(outcome, scheduled_for);

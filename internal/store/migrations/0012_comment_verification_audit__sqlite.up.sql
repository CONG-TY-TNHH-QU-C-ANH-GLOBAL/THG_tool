-- Manual human verification of comments (spec: specs/COMMENT_ASYNC_REVERIFY.md companion).
-- When async machine verification can't confirm a SUBMITTED comment (submitted_unverified)
-- but an operator opens Facebook and sees it posted by the right account, they may manually
-- confirm it. The confirmation APPENDS a 'succeeded'/'human_verified' correction to
-- action_ledger (never mutates the old row); this table is the audit trail — who confirmed,
-- when, the previous vs new effective outcome, and the correction ledger row id.
CREATE TABLE comment_verification_audit (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id                INTEGER NOT NULL,
    outbound_id           INTEGER NOT NULL,
    target_url            TEXT    NOT NULL DEFAULT '',
    account_id            INTEGER NOT NULL DEFAULT 0,
    verified_by_user_id   INTEGER NOT NULL DEFAULT 0,
    source                TEXT    NOT NULL DEFAULT '',  -- operator_manual_confirm
    previous_outcome      TEXT    NOT NULL DEFAULT '',  -- submitted_unverified
    new_effective_outcome TEXT    NOT NULL DEFAULT '',  -- succeeded
    correction_ledger_id  INTEGER NOT NULL DEFAULT 0,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_comment_verify_audit_org ON comment_verification_audit(org_id, outbound_id);

-- PR-M1: per-user copilot chat privacy.
-- prompt_logs had no user_id, so GetPromptHistoryForOrg returned EVERY member's
-- chat (commands + AI responses) to every member in the org. Add user_id so the
-- history can be scoped to the caller. user-typed rows carry the member's id;
-- system events (source='system') stay user_id=0 and remain visible to all as the
-- shared execution feed.
ALTER TABLE prompt_logs ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_prompt_logs_org_user ON prompt_logs(org_id, user_id, created_at);

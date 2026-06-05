-- PR-M1: per-user copilot chat privacy (Postgres variant).
-- Guarded so it is a no-op when prompt_logs has not been created on this
-- Postgres database yet (the legacy chat table originated on the SQLite side).
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'prompt_logs') THEN
    ALTER TABLE prompt_logs ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;
    CREATE INDEX IF NOT EXISTS idx_prompt_logs_org_user ON prompt_logs(org_id, user_id, created_at);
  END IF;
END $$;

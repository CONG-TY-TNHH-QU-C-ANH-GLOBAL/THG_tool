-- In-app notifications + soft invite revoke (SaaS UX Hardening PR-1).
--
-- notifications: minimal persistent in-app notification substrate.
--   user_id > 0  → visible to that user only (cross-org allowed: an
--                  invite notification reaches a user before they join).
--   user_id = 0  → org-wide, visible to the org's admins.
CREATE TABLE IF NOT EXISTS notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL DEFAULT 0,
  type TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '{}',
  read_at DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, read_at, created_at);
CREATE INDEX IF NOT EXISTS idx_notifications_org ON notifications(org_id, user_id, read_at, created_at);

-- Soft revoke: admins need pending|accepted|expired|revoked visibility.
-- DELETE destroyed the history; revoked_at keeps the row + the status.
ALTER TABLE org_invites ADD COLUMN revoked_at DATETIME;

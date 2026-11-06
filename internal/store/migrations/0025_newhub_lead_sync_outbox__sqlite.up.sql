-- Second, independent outbox for the NewHub CRM extension while the old CRM and
-- NewHub run side by side. Same shape as crm_lead_sync_outbox: one table per
-- receiver so a receiver that is down never delays the other one.
CREATE TABLE IF NOT EXISTS newhub_lead_sync_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_key TEXT NOT NULL UNIQUE,
  payload_json TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending'
    CHECK (state IN ('pending','sending','succeeded','failed','blocked')),
  attempts INTEGER NOT NULL DEFAULT 0,
  available_at INTEGER NOT NULL DEFAULT 0,
  locked_until INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_newhub_lead_sync_outbox_ready
  ON newhub_lead_sync_outbox(state, available_at, id);

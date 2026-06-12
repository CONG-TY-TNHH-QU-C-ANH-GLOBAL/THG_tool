-- Extension version policy + heartbeat enrichment (SaaS UX Hardening PR-4).
--
-- extension_policies: ONE platform-scoped row (id = 1) — the source of
-- truth for the version gate. Empty version fields fall back to the
-- compiled default (connectors.DefaultVersionPolicy) so a missing row
-- never disables the gate.
CREATE TABLE IF NOT EXISTS extension_policies (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  latest_version TEXT NOT NULL DEFAULT '',
  min_supported_version TEXT NOT NULL DEFAULT '',
  min_required_version TEXT NOT NULL DEFAULT '',
  release_channel TEXT NOT NULL DEFAULT 'stable',
  release_notes TEXT NOT NULL DEFAULT '',
  update_url TEXT NOT NULL DEFAULT '',
  update_instructions TEXT NOT NULL DEFAULT '',
  force_update_after DATETIME,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Heartbeat now reports build + channel alongside the manifest version.
ALTER TABLE agent_tokens ADD COLUMN build_number TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_tokens ADD COLUMN release_channel TEXT NOT NULL DEFAULT '';

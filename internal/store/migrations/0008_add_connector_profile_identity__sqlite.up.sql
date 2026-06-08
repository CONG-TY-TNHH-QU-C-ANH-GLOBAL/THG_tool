-- Facebook Automation Reliability Track — PR-C (Connector Registry / multi-account
-- per device). One physical machine can run MANY Chrome profiles, each a separate
-- connector bound to its own Facebook account. Chrome cannot expose a real
-- machine id or the profile name to an extension, so (founder decision):
--   browser_profile_id — a STABLE per-profile UUID the extension generates once and
--                        stores in chrome.storage.local (distinguishes profiles on
--                        the same machine). Sent every heartbeat.
--   machine_label      — a human label the admin types at pairing ("Máy A / FB
--                        chính"); the UI groups connectors by it. NO machine
--                        fingerprint (would be approximate + wrong).
ALTER TABLE agent_tokens ADD COLUMN browser_profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_tokens ADD COLUMN machine_label TEXT NOT NULL DEFAULT '';

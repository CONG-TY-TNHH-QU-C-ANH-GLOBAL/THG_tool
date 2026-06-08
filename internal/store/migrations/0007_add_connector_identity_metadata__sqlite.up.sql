-- Facebook Automation Reliability Track — PR-B (Identity Accuracy), part 2 / B3.
-- Connector heartbeat now reports HOW confident/where-from the live Facebook
-- identity was extracted, so readiness (PR-D) and the health board (PR-E) can
-- show "identity verified vs unknown" without trusting the display name.
--   identity_confidence:        'high' | 'low' | 'none'   (high = c_user cookie present)
--   identity_extraction_method: 'cookie_c_user' | 'dom_selector' | 'none'
--   identity_last_verified_at:  RFC3339 timestamp of the last c_user read ('' if never)
ALTER TABLE agent_tokens ADD COLUMN identity_confidence TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_tokens ADD COLUMN identity_extraction_method TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_tokens ADD COLUMN identity_last_verified_at TEXT NOT NULL DEFAULT '';

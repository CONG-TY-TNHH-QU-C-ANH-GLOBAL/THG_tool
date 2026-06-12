-- Admin assignment pause (SaaS UX Hardening PR-2b).
-- accounts.assignment_paused is the admin safety switch: when 1, no new
-- automation task may be queued for the account. It is a deliberate
-- operator control, distinct from cooldown/risk (pacing) and
-- actor_blocked (integrity). Checked as gate #0 in DecideCaps with the
-- typed reason assignment_paused_by_admin.
ALTER TABLE accounts ADD COLUMN assignment_paused INTEGER NOT NULL DEFAULT 0;

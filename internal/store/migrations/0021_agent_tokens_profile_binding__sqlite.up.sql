-- Extension Pairing ownership boundary: the Chrome profile
-- (agent_tokens.browser_profile_id == extension_installation_id) is the
-- connector boundary — NOT the physical device. The claim-time guard
-- (connector_pairing_guard.go) enforces "at most one active extension
-- connector per profile" procedurally; this index makes the invariant
-- STRUCTURAL so concurrent claims can never mint cross-user duplicates.
--
-- Pre-step: legacy data may already hold duplicate active bindings for one
-- profile (pairing predated the guard). Keep the binding that is provably
-- ALIVE — most recent last_seen (heartbeat), id as tie-break — and deactivate
-- the rest (same effect as dashboard disconnect; tokens stay for audit,
-- active=0). Keeping MAX(id) instead could kill the live token when a newer
-- orphan row exists (pairing response lost after the server INSERT).
UPDATE agent_tokens SET active = 0
WHERE active = 1
  AND kind = 'extension_connector'
  AND COALESCE(browser_profile_id, '') <> ''
  AND id NOT IN (
    SELECT id FROM (
      SELECT id, ROW_NUMBER() OVER (
        PARTITION BY browser_profile_id
        ORDER BY COALESCE(last_seen, '1970-01-01') DESC, id DESC
      ) AS rn
      FROM agent_tokens
      WHERE active = 1
        AND kind = 'extension_connector'
        AND COALESCE(browser_profile_id, '') <> ''
    ) ranked
    WHERE rn = 1
  );

CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_tokens_active_profile
ON agent_tokens(browser_profile_id)
WHERE active = 1 AND browser_profile_id <> '' AND kind = 'extension_connector';

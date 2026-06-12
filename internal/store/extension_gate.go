package store

import (
	"database/sql"
	"log"

	"github.com/thg/scraper/internal/store/connectors"
)

// Pre-queue extension version gate for outbound automation
// (SaaS UX Hardening PR-4). Composed at the store root because it spans
// three domains: identities (account row), connectors (policy +
// PickReadyConnector) and coordination (action_ledger audit).
//
// LedgerReasonExtensionBlocked is the typed audit reason written when a
// queue attempt is denied by the version gate. Never silent: every
// denial leaves a skipped ledger row.
const LedgerReasonExtensionBlocked = "blocked_by_extension_version"

// extensionGateForOutbound returns blocked=true when the account's
// connector runs a blocked extension version (update_required /
// unsupported). Other connector states (offline, identity unknown…) do
// NOT block queueing here — those are dispatch-time concerns and
// blocking them at queue time would change existing behavior.
//
// tenant-ok: cross-domain read (outbound gate -> identities/connectors).
func (s *Store) extensionGateForOutbound(tx *sql.Tx, accountID int64, msgType string) bool {
	var orgID int64
	var fbUserID string
	if err := tx.QueryRow(
		`SELECT COALESCE(org_id, 0), COALESCE(fb_user_id, '') FROM accounts WHERE id = ?`,
		accountID,
	).Scan(&orgID, &fbUserID); err != nil || orgID <= 0 {
		return false // unknown account → let downstream guards decide
	}
	policy, err := s.Connectors().GetExtensionPolicy()
	if err != nil {
		return false
	}
	conns, err := s.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return false
	}
	_, reason := connectors.PickReadyConnector(conns, accountID, fbUserID, policy)
	if reason != connectors.ConnExtensionUpdateRequired && reason != connectors.ConnExtensionUnsupported {
		return false
	}
	// Audit the denial in the action ledger (outcome=skipped) inside the
	// same queue transaction — blocked_by_extension_version is queryable
	// next to every other execution outcome. target_url is empty: the
	// gate fires before a target row exists.
	if _, err := tx.Exec(
		`INSERT INTO action_ledger
			(org_id, action_type, target_type, target_url, account_id, created_by, outbound_id,
			 performed_at, outcome, reason)
		 VALUES (?, ?, '', '', ?, 0, 0, CURRENT_TIMESTAMP, 'skipped', ?)`,
		orgID, msgType, accountID, LedgerReasonExtensionBlocked,
	); err != nil {
		log.Printf("[ExtensionGate] ledger insert failed org=%d account=%d: %v", orgID, accountID, err)
	}
	return true
}

// Domain: coordination (see internal/store/DOMAINS.md)
package coordination

import (
	"database/sql"
	"fmt"
	"strings"
)

// RecordBlockedTx appends a SKIPPED action_ledger row for an action
// denied by a pre-queue gate (e.g. blocked_by_extension_version),
// inside the caller's open transaction so the audit commits atomically
// with the denial. target_url is empty by design: pre-queue gates fire
// before a target row exists.
//
// Coordination owns ALL action_ledger writes (topology guard §4) —
// gate layers call this instead of issuing their own INSERT.
func RecordBlockedTx(tx *sql.Tx, orgID, accountID int64, actionType, reason string) error {
	if tx == nil || orgID <= 0 || strings.TrimSpace(actionType) == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("blocked ledger row requires tx, org_id, action_type, reason")
	}
	_, err := tx.Exec(
		`INSERT INTO action_ledger
			(org_id, action_type, target_type, target_url, account_id, created_by, outbound_id,
			 performed_at, outcome, reason)
		 VALUES (?, ?, ?, '', ?, 0, 0, CURRENT_TIMESTAMP, ?, ?)`,
		orgID, actionType, targetTypeFromAction(actionType), accountID, LedgerOutcomeSkipped, reason,
	)
	return err
}

// Domain: foundation — deterministic ExecutionContext (Organic Sales Network)
package store

import (
	"fmt"

	"github.com/thg/scraper/internal/models"
)

// GetUserDefaultAccount returns the member's chosen default execution account
// for an org, or 0 when none is set. Part of the deterministic ExecutionContext:
// outbound routing prefers this over any heuristic account guessing.
func (s *Store) GetUserDefaultAccount(orgID, userID int64) int64 {
	if orgID <= 0 || userID <= 0 {
		return 0
	}
	var id int64
	_ = s.db.QueryRow(
		`SELECT default_account_id FROM user_execution_context WHERE org_id = ? AND user_id = ?`,
		orgID, userID,
	).Scan(&id)
	return id
}

// SetUserDefaultAccount records the member's default execution account. The
// account must belong to the org AND be owned by the member (RBAC). accountID=0
// clears the default.
func (s *Store) SetUserDefaultAccount(orgID, userID, accountID int64, role string) error {
	if orgID <= 0 || userID <= 0 {
		return fmt.Errorf("org_id and user_id are required")
	}
	if accountID > 0 {
		acc, err := s.Identities().GetAccountForOrg(accountID, orgID)
		if err != nil || acc == nil {
			return fmt.Errorf("account #%d not found in org %d", accountID, orgID)
		}
		if !models.IsAccountOwnerAllowed(acc, userID, role) {
			return fmt.Errorf("you do not own account #%d", accountID)
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO user_execution_context (org_id, user_id, default_account_id, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(org_id, user_id) DO UPDATE SET
		   default_account_id = excluded.default_account_id, updated_at = CURRENT_TIMESTAMP`,
		orgID, userID, accountID,
	)
	return err
}

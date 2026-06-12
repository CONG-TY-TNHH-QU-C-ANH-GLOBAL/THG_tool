// Domain: identities (see internal/store/DOMAINS.md)
package identities

import "fmt"

// SetAccountAssignmentPaused flips the admin assignment-pause switch
// (PR-2b). org-scoped: the UPDATE matches both id and org_id so a
// cross-tenant id can never be paused/resumed. Returns an error when no
// row matched (wrong org or missing account) so handlers can 404
// instead of reporting a silent success.
func (s *Store) SetAccountAssignmentPaused(accountID, orgID int64, paused bool) error {
	if accountID <= 0 || orgID <= 0 {
		return fmt.Errorf("account id and org id are required")
	}
	v := 0
	if paused {
		v = 1
	}
	res, err := s.db.Exec(
		`UPDATE accounts SET assignment_paused = ? WHERE id = ? AND org_id = ?`,
		v, accountID, orgID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("account not found in organization")
	}
	return nil
}

// AccountAssignmentPaused reads the admin pause flag. Missing account
// reads as paused=false; callers that need existence checks use
// GetAccountForOrg first.
func (s *Store) AccountAssignmentPaused(accountID int64) (bool, error) {
	var v int
	err := s.db.QueryRow(
		`SELECT COALESCE(assignment_paused, 0) FROM accounts WHERE id = ?`, accountID,
	).Scan(&v)
	if err != nil {
		return false, nil // sql.ErrNoRows and scan errors read as not-paused
	}
	return v == 1, nil
}

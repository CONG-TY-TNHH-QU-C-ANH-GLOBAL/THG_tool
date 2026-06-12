package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/thg/scraper/internal/models"
)

// Workspace detach (membership-vulnerability fix): removing a staff
// member from a workspace must NEVER destroy the global user record or
// their login credentials. Detach resets the membership only; the user
// can still log in and accept/join another workspace later.

// ErrLastAdmin blocks removing/leaving when the target is the last
// active admin of the workspace — a workspace must never be orphaned.
var ErrLastAdmin = errors.New("last admin cannot be removed from workspace")

// DetachUserFromOrg removes ONE user's membership in orgID:
//   - users row is preserved (id, email, password hash, refresh tokens);
//     only org_id is reset to 0 and role to the orgless signup default
//     ('admin' — so the user can create or join a workspace again).
//   - the user's assigned Facebook accounts in the org are
//     assignment-paused so no automation keeps running unowned; the
//     workspace admin can resume/reassign them deliberately.
//   - guarded: the last active admin cannot be detached (ErrLastAdmin).
//
// Single tx so membership reset and automation pause commit together.
func (s *Store) DetachUserFromOrg(userID, orgID int64) error {
	if userID <= 0 || orgID <= 0 {
		return fmt.Errorf("user id and org id are required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var role string
	err = tx.QueryRow(
		`SELECT role FROM users WHERE id = ? AND org_id = ?`, userID, orgID,
	).Scan(&role)
	if err == sql.ErrNoRows {
		return fmt.Errorf("user is not a member of this workspace")
	}
	if err != nil {
		return err
	}
	if models.UserRole(role) == models.RoleAdmin {
		var otherAdmins int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM users
			  WHERE org_id = ? AND role = 'admin' AND active = 1 AND id != ?`,
			orgID, userID,
		).Scan(&otherAdmins); err != nil {
			return err
		}
		if otherAdmins == 0 {
			return ErrLastAdmin
		}
	}

	if _, err := tx.Exec(
		`UPDATE users SET org_id = 0, role = 'admin' WHERE id = ?`, userID,
	); err != nil {
		return err
	}
	// Safety: pause automation assignment on the leaver's accounts so the
	// workspace never keeps executing under an unowned identity.
	if _, err := tx.Exec(
		`UPDATE accounts SET assignment_paused = 1
		  WHERE org_id = ? AND assigned_user_id = ?`, orgID, userID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// Domain: connectors (see internal/store/DOMAINS.md)
package connectors

import (
	"database/sql"
	"strings"
)

// isProfileUniqueViolation detects a violation of uq_agent_tokens_active_profile
// (migration 0021). modernc/sqlite reports column-index violations as
// "UNIQUE constraint failed: agent_tokens.browser_profile_id" (no index name);
// Postgres reports the index name — match both.
func isProfileUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "uq_agent_tokens_active_profile") ||
		strings.Contains(msg, "agent_tokens.browser_profile_id")
}

// guardBrowserProfileBindingTx enforces the ownership boundary of a Chrome
// profile (stable browser_profile_id == extension_installation_id) at claim
// time, inside the claim transaction:
//
//   - bound to ANOTHER user            → ErrDevicePairedToAnotherUser
//   - bound to ANOTHER workspace       → ErrDevicePairedToAnotherWorkspace
//   - bound to the SAME user+workspace → explicit re-pair: revoke the old
//     binding(s) so exactly one active connector represents the profile.
//
// The physical device is NOT the boundary — many Chrome profiles (and many THG
// users) share one laptop. Only the Chrome profile binds. An empty
// browserProfileID (legacy extension build) skips the guard for backward
// compatibility.
func guardBrowserProfileBindingTx(tx *sql.Tx, browserProfileID string, orgID, createdBy int64) error {
	if browserProfileID == "" {
		return nil
	}
	// The literal browser_profile_id <> '' term is redundant with the bound
	// parameter but lets SQLite prove the partial-index predicate and use
	// uq_agent_tokens_active_profile instead of scanning all token rows.
	rows, err := tx.Query(
		`SELECT id, org_id, created_by FROM agent_tokens
		 WHERE browser_profile_id = ? AND browser_profile_id <> ''
		   AND kind = 'extension_connector' AND active = 1`,
		browserProfileID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var samePairIDs []int64
	otherUser := false
	otherWorkspace := false
	for rows.Next() {
		var id, rowOrg, rowUser int64
		if err := rows.Scan(&id, &rowOrg, &rowUser); err != nil {
			return err
		}
		switch {
		case rowUser != createdBy:
			otherUser = true
		case rowOrg != orgID:
			otherWorkspace = true
		default:
			samePairIDs = append(samePairIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if otherUser {
		return ErrDevicePairedToAnotherUser
	}
	if otherWorkspace {
		return ErrDevicePairedToAnotherWorkspace
	}
	for _, id := range samePairIDs {
		if _, err := tx.Exec(`UPDATE agent_tokens SET active = 0 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

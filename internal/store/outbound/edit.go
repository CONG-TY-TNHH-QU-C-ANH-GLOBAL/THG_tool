package outbound

import (
	"database/sql"
)

// EditContent updates planned content only within one tenant. PR-2
// (V2 tenant isolation): editing content while the row is already
// executing or finished is intentionally a no-op — the row affected
// count is zero and the caller gets sql.ErrNoRows so the UI can
// surface "this comment is already in flight".
func (s *Store) EditContent(orgID, id int64, content string) error {
	res, err := s.db.Exec(`UPDATE outbound_messages SET content = ? WHERE id = ? AND org_id = ? AND execution_state = 'planned'`, content, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete deletes an outbound message within one tenant.
//
// Soft semantics note: the row in outbound_messages is gone, but the
// execution_attempts ledger retains every plan/claim/finalize/reset
// transition that ever happened against this outbound_id. That audit
// trail is intentional — operator replay can reconstruct what was
// attempted even if the source row was deleted.
func (s *Store) Delete(orgID, id int64) error {
	res, err := s.db.Exec(`DELETE FROM outbound_messages WHERE id = ? AND org_id = ?`, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

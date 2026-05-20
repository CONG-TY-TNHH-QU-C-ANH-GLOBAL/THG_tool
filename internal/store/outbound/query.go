package outbound

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// readColumns is the single source of truth for the column list every
// read path SELECTs. Centralising it means a schema addition is a
// single-edit change across every Get* helper.
const readColumns = `id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), execution_state, COALESCE(verification_outcome,''), ai_model, COALESCE(sent_at, ''), created_at, COALESCE(execution_id, '')`

// scanRow parses one row from any query selecting [readColumns] in
// order. Centralised so adding/reordering a column is a single-edit
// change across every Get* helper.
func scanRow(rows *sql.Rows) (*models.OutboundMessage, error) {
	var m models.OutboundMessage
	var sentAt string
	var verifOutcome string
	err := rows.Scan(&m.ID, &m.OrgID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
		&m.Content, &m.Context, &m.ImagePath, &m.ExecutionState, &verifOutcome, &m.AIModel, &sentAt, &m.CreatedAt, &m.ExecutionID)
	if err != nil {
		return nil, err
	}
	m.VerificationOutcome = models.VerificationOutcome(verifOutcome)
	if sentAt != "" {
		m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
	}
	return &m, nil
}

// Get returns one tenant-scoped outbound message. Returns sql.ErrNoRows
// if the row doesn't exist or belongs to a different tenant.
func (s *Store) Get(orgID, id int64) (*models.OutboundMessage, error) {
	var m models.OutboundMessage
	var sentAt string
	var verifOutcome string
	err := s.db.QueryRow(
		`SELECT `+readColumns+`
		FROM outbound_messages WHERE id = ? AND org_id = ?`, id, orgID,
	).Scan(&m.ID, &m.OrgID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
		&m.Content, &m.Context, &m.ImagePath, &m.ExecutionState, &verifOutcome, &m.AIModel, &sentAt, &m.CreatedAt, &m.ExecutionID)
	if err != nil {
		return nil, err
	}
	m.VerificationOutcome = models.VerificationOutcome(verifOutcome)
	if sentAt != "" {
		m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
	}
	return &m, nil
}

// ListByState returns outbound messages whose execution_state matches
// the filter. Pass execState="" to skip the state filter; pass
// msgType="" to skip the type filter.
func (s *Store) ListByState(orgID int64, execState models.ExecutionState, msgType string, limit int) ([]models.OutboundMessage, error) {
	return s.list(orgID, execState, msgType, limit)
}

// ListByLegacyStatus is the back-compat read for dashboards still
// passing the legacy single-string status ('approved', 'sending',
// 'sent', 'failed', 'expired'). Maps the input to an execution_state
// filter. Empty input returns the full set.
func (s *Store) ListByLegacyStatus(orgID int64, legacyStatus, msgType string, limit int) ([]models.OutboundMessage, error) {
	var execState models.ExecutionState
	switch strings.ToLower(strings.TrimSpace(legacyStatus)) {
	case "approved", "planned":
		execState = models.ExecPlanned
	case "sending", "executing":
		execState = models.ExecExecuting
	case "sent", "finished":
		execState = models.ExecFinished
	case "expired":
		execState = models.ExecExpired
	case "failed", "rejected":
		// Legacy lump: finished+non-verified rows. The V2-aware
		// dashboard filter pills already split via verification_outcome.
		execState = models.ExecFinished
	}
	return s.list(orgID, execState, msgType, limit)
}

// list is the shared scan implementation behind ListByState and
// ListByLegacyStatus.
// tenant-ok: the SELECT body has no static WHERE clause because the
// query is composed dynamically below. The first appended clause is
// always `org_id = ?` (line `clauses := []string{"org_id = ?"}`), so
// the executed SQL is always tenant-scoped — the linter just cannot
// statically see the dynamic concatenation.
func (s *Store) list(orgID int64, execState models.ExecutionState, msgType string, limit int) ([]models.OutboundMessage, error) {
	query := `SELECT ` + readColumns + `
		FROM outbound_messages`
	var args []any
	clauses := []string{"org_id = ?"}
	args = append(args, orgID)
	if execState != "" {
		clauses = append(clauses, "execution_state = ?")
		args = append(args, string(execState))
	}
	if msgType != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, msgType)
	}
	query += " WHERE " + strings.Join(clauses, " AND ")
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanRow(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// VerifiedGroupPostsWithin returns group_post messages for one tenant
// that were successfully delivered (within last N days). "Sent" is
// defined post-PR-1 as
// (execution_state='finished' AND verification_outcome='verified_success').
func (s *Store) VerifiedGroupPostsWithin(orgID int64, withinDays int) ([]models.OutboundMessage, error) {
	rows, err := s.db.Query(
		`SELECT `+readColumns+`
		FROM outbound_messages
		WHERE org_id = ? AND type = 'group_post'
		  AND execution_state = 'finished' AND verification_outcome = 'verified_success'
		  AND created_at >= datetime('now', ?)
		ORDER BY created_at DESC`,
		orgID, fmt.Sprintf("-%d days", withinDays),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanRow(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// CountByState returns tenant-scoped counts grouped by the
// (execution_state, verification_outcome) pair. The returned map keys
// are the legacy single-string status values for back-compat with
// existing dashboard JSON consumers — derived from
// (execution_state, verification_outcome) via the same projection rule
// the API has always exposed.
func (s *Store) CountByState(orgID int64) (map[string]int, error) {
	rows, err := s.db.Query(
		`SELECT execution_state, COALESCE(verification_outcome,''), COUNT(*)
		 FROM outbound_messages
		 WHERE org_id = ?
		 GROUP BY execution_state, verification_outcome`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var state, outcome string
		var count int
		if err := rows.Scan(&state, &outcome, &count); err == nil {
			// Project (state, outcome) onto a flat key for back-compat.
			// "planned" / "executing" / "finished:verified_success" /
			// "finished:context_drift" / "expired" / etc.
			key := state
			if outcome != "" {
				key = state + ":" + outcome
			}
			counts[key] = count
		}
	}
	return counts, nil
}

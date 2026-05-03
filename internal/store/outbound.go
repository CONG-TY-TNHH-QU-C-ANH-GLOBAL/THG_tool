package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// InsertOutboundMessage creates a new outbound message in the queue.
func (s *Store) InsertOutboundMessage(msg *models.OutboundMessage) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO outbound_messages (org_id, type, platform, account_id, target_url, target_name, content, context, image_path, status, ai_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.OrgID, msg.Type, msg.Platform, msg.AccountID, msg.TargetURL, msg.TargetName, msg.Content, msg.Context, msg.ImagePath, msg.Status, msg.AIModel,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// OutboundGuardDecision is the queue-level safety check result for automated
// comments/inbox messages. AI can propose actions, but this guard is the final
// production gate before anything reaches an executable outbox state.
type OutboundGuardDecision struct {
	Allowed        bool
	Reason         string
	ExistingID     int64
	LastOutboundAt time.Time
	LastInboundAt  time.Time
}

// CanQueueOutboundForOrg prevents repeated comments/messages against the same
// post/profile unless the lead has replied and the conversation needs service.
func (s *Store) CanQueueOutboundForOrg(orgID int64, msgType, targetURL, profileURL string, cooldown time.Duration) (OutboundGuardDecision, error) {
	msgType = strings.TrimSpace(strings.ToLower(msgType))
	targetURL = strings.TrimSpace(targetURL)
	profileURL = strings.TrimSpace(profileURL)
	if targetURL == "" {
		return OutboundGuardDecision{Allowed: false, Reason: "missing_target_url"}, nil
	}
	if cooldown <= 0 {
		cooldown = 24 * time.Hour
	}

	var existingID int64
	var status string
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, status, COALESCE(sent_at, created_at)
		 FROM outbound_messages
		 WHERE org_id = ? AND type = ? AND target_url = ?
		   AND status NOT IN ('failed','rejected')
		 ORDER BY created_at DESC LIMIT 1`,
		orgID, msgType, targetURL,
	).Scan(&existingID, &status, &createdAt)
	if err != nil && err != sql.ErrNoRows {
		return OutboundGuardDecision{}, err
	}
	if err == nil {
		lastAt := parseSQLiteTime(createdAt)
		if msgType == "comment" || status == string(models.OutboundDraft) || status == string(models.OutboundApproved) {
			return OutboundGuardDecision{Allowed: false, Reason: "duplicate_outbound_target", ExistingID: existingID, LastOutboundAt: lastAt}, nil
		}
		if time.Since(lastAt) < cooldown {
			return OutboundGuardDecision{Allowed: false, Reason: "outbound_cooldown_active", ExistingID: existingID, LastOutboundAt: lastAt}, nil
		}
	}

	if msgType == "inbox" {
		if profileURL == "" {
			profileURL = targetURL
		}
		if thread, err := s.GetThreadByProfileForOrg(orgID, profileURL); err == nil && thread != nil {
			if thread.Status == "closed" || thread.Status == "converted" {
				return OutboundGuardDecision{Allowed: false, Reason: "conversation_closed", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
			if !thread.LastInboundAt.IsZero() && thread.LastInboundAt.After(thread.LastOutboundAt) {
				return OutboundGuardDecision{Allowed: true, Reason: "lead_replied", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
			if !thread.LastOutboundAt.IsZero() && time.Since(thread.LastOutboundAt) < cooldown {
				return OutboundGuardDecision{Allowed: false, Reason: "awaiting_reply_cooldown", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
		} else if err != nil && err != sql.ErrNoRows {
			return OutboundGuardDecision{}, err
		}
	}

	return OutboundGuardDecision{Allowed: true, Reason: "ok"}, nil
}

// GetOutboundByStatus returns outbound messages filtered by status.
func (s *Store) GetOutboundByStatus(status string, limit int) ([]models.OutboundMessage, error) {
	query := `SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at
		FROM outbound_messages`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanOutboundMessage(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// GetOutboundByStatusForOrg returns outbound messages for one tenant.
func (s *Store) GetOutboundByStatusForOrg(orgID int64, status string, limit int) ([]models.OutboundMessage, error) {
	return s.GetOutboundByFilterForOrg(orgID, status, "", limit)
}

// GetOutboundByFilter returns outbound messages filtered by optional status and/or type.
func (s *Store) GetOutboundByFilter(status, msgType string, limit int) ([]models.OutboundMessage, error) {
	query := `SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at
		FROM outbound_messages`
	var args []any
	var clauses []string
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if msgType != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, msgType)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanOutboundMessage(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// GetOutboundByFilterForOrg returns tenant-scoped outbound messages.
func (s *Store) GetOutboundByFilterForOrg(orgID int64, status, msgType string, limit int) ([]models.OutboundMessage, error) {
	query := `SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at
		FROM outbound_messages`
	var args []any
	clauses := []string{"org_id = ?"}
	args = append(args, orgID)
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
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
		m, err := scanOutboundMessage(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// GetSentGroupPosts returns group_post messages that were successfully sent (within last N days).
func (s *Store) GetSentGroupPosts(withinDays int) ([]models.OutboundMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
			COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at
		FROM outbound_messages
		WHERE type = 'group_post' AND status IN ('sent', 'approved')
		  AND created_at >= datetime('now', ?)
		ORDER BY created_at DESC`,
		fmt.Sprintf("-%d days", withinDays),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanOutboundMessage(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// GetOutbound returns a single outbound message by ID.
func (s *Store) GetOutbound(id int64) (*models.OutboundMessage, error) {
	var m models.OutboundMessage
	var sentAt string
	err := s.db.QueryRow(
		`SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at
		FROM outbound_messages WHERE id = ?`, id,
	).Scan(&m.ID, &m.OrgID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
		&m.Content, &m.Context, &m.ImagePath, &m.Status, &m.AIModel, &sentAt, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	if sentAt != "" {
		m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
	}
	return &m, nil
}

// GetOutboundForOrg returns one tenant-scoped outbound message.
func (s *Store) GetOutboundForOrg(orgID, id int64) (*models.OutboundMessage, error) {
	msg, err := s.GetOutbound(id)
	if err != nil {
		return nil, err
	}
	if msg.OrgID != orgID {
		return nil, sql.ErrNoRows
	}
	return msg, nil
}

// UpdateOutboundStatus updates the status of an outbound message.
func (s *Store) UpdateOutboundStatus(id int64, status models.OutboundStatus) error {
	query := `UPDATE outbound_messages SET status = ? WHERE id = ?`
	if status == models.OutboundSent {
		query = `UPDATE outbound_messages SET status = ?, sent_at = CURRENT_TIMESTAMP WHERE id = ?`
	}
	_, err := s.db.Exec(query, status, id)
	return err
}

// UpdateOutboundStatusForOrg updates status only when the message belongs to the tenant.
func (s *Store) UpdateOutboundStatusForOrg(orgID, id int64, status models.OutboundStatus) error {
	query := `UPDATE outbound_messages SET status = ? WHERE id = ? AND org_id = ?`
	if status == models.OutboundSent {
		query = `UPDATE outbound_messages SET status = ?, sent_at = CURRENT_TIMESTAMP WHERE id = ? AND org_id = ?`
	}
	res, err := s.db.Exec(query, status, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateOutboundContent updates the content of a draft message.
func (s *Store) UpdateOutboundContent(id int64, content string) error {
	_, err := s.db.Exec(`UPDATE outbound_messages SET content = ? WHERE id = ? AND status = 'draft'`, content, id)
	return err
}

// UpdateOutboundContentForOrg updates draft content only within one tenant.
func (s *Store) UpdateOutboundContentForOrg(orgID, id int64, content string) error {
	res, err := s.db.Exec(`UPDATE outbound_messages SET content = ? WHERE id = ? AND org_id = ? AND status = 'draft'`, content, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteOutbound deletes an outbound message.
func (s *Store) DeleteOutbound(id int64) error {
	_, err := s.db.Exec(`DELETE FROM outbound_messages WHERE id = ?`, id)
	return err
}

// DeleteOutboundForOrg deletes an outbound message only within one tenant.
func (s *Store) DeleteOutboundForOrg(orgID, id int64) error {
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

// CountOutboundByStatus returns counts for each status.
func (s *Store) CountOutboundByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM outbound_messages GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err == nil {
			counts[status] = count
		}
	}
	return counts, nil
}

// CountOutboundByStatusForOrg returns tenant-scoped status counts.
func (s *Store) CountOutboundByStatusForOrg(orgID int64) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM outbound_messages WHERE org_id = ? GROUP BY status`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err == nil {
			counts[status] = count
		}
	}
	return counts, nil
}

func scanOutboundMessage(rows *sql.Rows) (*models.OutboundMessage, error) {
	var m models.OutboundMessage
	var sentAt string
	err := rows.Scan(&m.ID, &m.OrgID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
		&m.Content, &m.Context, &m.ImagePath, &m.Status, &m.AIModel, &sentAt, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	if sentAt != "" {
		m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
	}
	return &m, nil
}

// Domain: prompts (see internal/store/DOMAINS.md)
package prompts

import (
	"database/sql"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// InsertScanLog records a scan cycle.
func (s *Store) InsertScanLog(log *models.ScanLog) error {
	_, err := s.db.Exec(
		`INSERT INTO scan_logs (platform, group_count, post_count, lead_count, duration, errors) VALUES (?, ?, ?, ?, ?, ?)`,
		log.Platform, log.GroupCount, log.PostCount, log.LeadCount, log.Duration, log.Errors,
	)
	return err
}

// InsertInboxMessage inserts a new inbox message.
func (s *Store) InsertInboxMessage(m *models.InboxMessage) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO inbox_messages (platform, sender, sender_url, content, is_read, received_at) VALUES (?, ?, ?, ?, ?, ?)`,
		m.Platform, m.Sender, m.SenderURL, m.Content, m.IsRead, m.ReceivedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertPromptLog records an AI prompt interaction.
//
// Watchpoint B: writes routing_decision_json alongside the legacy fields.
// Default '{}' when the caller didn't construct a decision — keeps the
// column always-valid JSON so dashboards can json.Unmarshal without
// per-row error handling.
func (s *Store) InsertPromptLog(p *models.PromptLog) error {
	decisionJSON := strings.TrimSpace(p.RoutingDecisionJSON)
	if decisionJSON == "" {
		decisionJSON = "{}"
	}
	_, err := s.db.Exec(
		`INSERT INTO prompt_logs (org_id, account_id, user_id, source, user_prompt, ai_response, action_taken, action_args, success, routing_decision_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.OrgID, p.AccountID, p.UserID, p.Source, p.UserPrompt, p.AIResponse, p.ActionTaken, p.ActionArgs, p.Success, decisionJSON,
	)
	return err
}

// InsertSystemPromptLog stores connector and automation updates in the same
// prompt history table so the dashboard chat can show crawl/outbox events.
func (s *Store) InsertSystemPromptLog(orgID, accountID int64, message, action, args string, success bool) error {
	return s.InsertPromptLog(&models.PromptLog{
		OrgID:       orgID,
		AccountID:   accountID,
		Source:      "system",
		UserPrompt:  "",
		AIResponse:  message,
		ActionTaken: action,
		ActionArgs:  args,
		Success:     success,
	})
}

// GetPromptHistory returns recent prompt logs.
func (s *Store) GetPromptHistory(limit int) ([]models.PromptLog, error) {
	rows, err := s.db.Query(
		`SELECT id, org_id, account_id, source, user_prompt, ai_response, action_taken, action_args, success, created_at FROM prompt_logs ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.PromptLog
	for rows.Next() {
		var p models.PromptLog
		if err := rows.Scan(&p.ID, &p.OrgID, &p.AccountID, &p.Source, &p.UserPrompt, &p.AIResponse, &p.ActionTaken, &p.ActionArgs, &p.Success, &p.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, p)
	}
	return logs, nil
}

// GetPromptHistoryForOrg returns recent prompt logs for one workspace, scoped to
// ONE member's copilot chat (PR-M1). It returns the caller's own user-typed
// prompts PLUS the shared system feed (source='system', user_id=0 — crawl/outbound
// status events that belong to the whole workspace, not a private conversation).
// Other members' typed prompts are NOT returned, so the chat is private per user.
//
// userID<=0 (legacy/unauthenticated) falls back to the pre-PR-M1 behaviour of
// returning user_id=0 rows + system, which is the safe no-identity default.
func (s *Store) GetPromptHistoryForOrg(orgID, userID int64, limit int) ([]models.PromptLog, error) {
	rows, err := s.db.Query(
		`SELECT id, org_id, account_id, source, user_prompt, ai_response, action_taken, action_args, success, created_at
		 FROM prompt_logs
		 WHERE org_id = ? AND (user_id = ? OR source = 'system')
		 ORDER BY created_at DESC
		 LIMIT ?`,
		orgID, userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.PromptLog
	for rows.Next() {
		var p models.PromptLog
		if err := rows.Scan(&p.ID, &p.OrgID, &p.AccountID, &p.Source, &p.UserPrompt, &p.AIResponse, &p.ActionTaken, &p.ActionArgs, &p.Success, &p.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, p)
	}
	return logs, rows.Err()
}

// DeletePromptLogForOrgUser deletes one chat row, scoped to the OWNING member
// (PR-M1). A member can only delete their own copilot messages, never another
// member's or the shared system feed.
func (s *Store) DeletePromptLogForOrgUser(orgID, userID, id int64) error {
	res, err := s.db.Exec(`DELETE FROM prompt_logs WHERE id = ? AND org_id = ? AND user_id = ?`, id, orgID, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteAllPromptLogsForUser clears one member's OWN copilot chat (PR-M1). It
// does NOT touch other members' chats or the shared system feed (user_id=0).
func (s *Store) DeleteAllPromptLogsForUser(orgID, userID int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM prompt_logs WHERE org_id = ? AND user_id = ? AND source != 'system'`, orgID, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InsertMemory stores a new learned pattern.
func (s *Store) InsertMemory(m *models.AIMemory) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO ai_memory (prompt_hash, category, user_prompt, best_action, best_args, use_count, success_rate) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.PromptHash, m.Category, m.UserPrompt, m.BestAction, m.BestArgs, m.UseCount, m.SuccessRate,
	)
	return err
}

// GetMemoryByHash returns a memory entry by prompt hash.
func (s *Store) GetMemoryByHash(hash string) (*models.AIMemory, error) {
	var m models.AIMemory
	err := s.db.QueryRow(
		`SELECT id, prompt_hash, category, user_prompt, best_action, best_args, use_count, success_rate, created_at, updated_at FROM ai_memory WHERE prompt_hash = ?`, hash,
	).Scan(&m.ID, &m.PromptHash, &m.Category, &m.UserPrompt, &m.BestAction, &m.BestArgs, &m.UseCount, &m.SuccessRate, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetRelevantMemories returns top memories sorted by success rate and usage.
func (s *Store) GetRelevantMemories(limit int) ([]models.AIMemory, error) {
	rows, err := s.db.Query(
		`SELECT id, prompt_hash, category, user_prompt, best_action, best_args, use_count, success_rate, created_at, updated_at FROM ai_memory ORDER BY use_count DESC, success_rate DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []models.AIMemory
	for rows.Next() {
		var m models.AIMemory
		if err := rows.Scan(&m.ID, &m.PromptHash, &m.Category, &m.UserPrompt, &m.BestAction, &m.BestArgs, &m.UseCount, &m.SuccessRate, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// UpdateMemoryUsage increments usage count and updates success rate.
func (s *Store) UpdateMemoryUsage(id int64, success bool) error {
	if success {
		_, err := s.db.Exec(`UPDATE ai_memory SET use_count = use_count + 1, success_rate = (success_rate * use_count + 1.0) / (use_count + 1), updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
		return err
	}
	_, err := s.db.Exec(`UPDATE ai_memory SET use_count = use_count + 1, success_rate = (success_rate * use_count) / (use_count + 1), updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

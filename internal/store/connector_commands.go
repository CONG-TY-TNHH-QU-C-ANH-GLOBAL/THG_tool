package store

import (
	"database/sql"
	"strings"
	"time"
)

type ConnectorCommand struct {
	ID          int64      `json:"id"`
	OrgID       int64      `json:"org_id"`
	AccountID   int64      `json:"account_id"`
	AgentID     int64      `json:"agent_id"`
	Type        string     `json:"type"`
	PayloadJSON string     `json:"payload_json"`
	Status      string     `json:"status"`
	ErrorMsg    string     `json:"error_msg"`
	CreatedBy   int64      `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (s *Store) CreateConnectorCommand(orgID, accountID, agentID, createdBy int64, typ, payloadJSON string) (int64, error) {
	typ = strings.TrimSpace(typ)
	if payloadJSON == "" {
		payloadJSON = "{}"
	}
	res, err := s.db.Exec(
		`INSERT INTO connector_commands
			(org_id, account_id, agent_id, type, payload_json, status, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', ?, CURRENT_TIMESTAMP)`,
		orgID, accountID, agentID, typ, payloadJSON, createdBy,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ClaimPendingConnectorCommands(orgID, agentID int64, limit int) ([]ConnectorCommand, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, org_id, account_id, agent_id, type, payload_json, status, COALESCE(error_msg,''), created_by, created_at
		 FROM connector_commands
		 WHERE org_id = ?
		   AND status = 'pending'
		   AND (agent_id = ? OR agent_id = 0)
		 ORDER BY id ASC
		 LIMIT ?`,
		orgID, agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []ConnectorCommand
	var ids []any
	for rows.Next() {
		var cmd ConnectorCommand
		var createdAt string
		if err := rows.Scan(&cmd.ID, &cmd.OrgID, &cmd.AccountID, &cmd.AgentID, &cmd.Type, &cmd.PayloadJSON, &cmd.Status, &cmd.ErrorMsg, &cmd.CreatedBy, &createdAt); err != nil {
			return nil, err
		}
		if parsed, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
			cmd.CreatedAt = parsed
		} else if parsed, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
			cmd.CreatedAt = parsed
		}
		commands = append(commands, cmd)
		ids = append(ids, cmd.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return commands, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := append([]any{agentID}, ids...)
	_, err = s.db.Exec(
		`UPDATE connector_commands
		 SET status = 'claimed', agent_id = CASE WHEN agent_id = 0 THEN ? ELSE agent_id END, claimed_at = CURRENT_TIMESTAMP
		 WHERE id IN (`+placeholders+`) AND status = 'pending'`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	return commands, nil
}

func (s *Store) CompleteConnectorCommand(id, agentID int64, errorMsg string) error {
	status := "done"
	errorMsg = strings.TrimSpace(errorMsg)
	if errorMsg != "" {
		status = "failed"
	}
	res, err := s.db.Exec(
		`UPDATE connector_commands
		 SET status = ?, error_msg = ?, completed_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND (agent_id = ? OR agent_id = 0)`,
		status, errorMsg, id, agentID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RecentConnectorCommands(orgID, accountID int64, limit int) ([]ConnectorCommand, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	query := `SELECT id, org_id, account_id, agent_id, type, payload_json, status, COALESCE(error_msg,''), created_by, created_at, claimed_at, completed_at
		FROM connector_commands
		WHERE org_id = ?`
	args := []any{orgID}
	if accountID > 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []ConnectorCommand
	for rows.Next() {
		var cmd ConnectorCommand
		var createdAt string
		var claimedAt, completedAt sql.NullString
		if err := rows.Scan(&cmd.ID, &cmd.OrgID, &cmd.AccountID, &cmd.AgentID, &cmd.Type, &cmd.PayloadJSON, &cmd.Status, &cmd.ErrorMsg, &cmd.CreatedBy, &createdAt, &claimedAt, &completedAt); err != nil {
			return nil, err
		}
		cmd.CreatedAt = parseConnectorCommandTime(createdAt)
		if claimedAt.Valid {
			if t := parseConnectorCommandTime(claimedAt.String); !t.IsZero() {
				cmd.ClaimedAt = &t
			}
		}
		if completedAt.Valid {
			if t := parseConnectorCommandTime(completedAt.String); !t.IsZero() {
				cmd.CompletedAt = &t
			}
		}
		commands = append(commands, cmd)
	}
	return commands, rows.Err()
}

// HasRecentConnectorCommand returns true if a command of the given type was
// created for this org+account within the specified window. Used to avoid
// sending duplicate control commands (e.g. window_control:minimize).
func (s *Store) HasRecentConnectorCommand(orgID, accountID int64, typ string, within time.Duration) bool {
	if within <= 0 {
		within = time.Hour
	}
	cutoff := time.Now().UTC().Add(-within).Format("2006-01-02 15:04:05")
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM connector_commands
		 WHERE org_id = ? AND account_id = ? AND type = ? AND created_at >= ?`,
		orgID, accountID, strings.TrimSpace(typ), cutoff,
	).Scan(&count)
	return err == nil && count > 0
}

func parseConnectorCommandTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return parsed
	}
	return time.Time{}
}

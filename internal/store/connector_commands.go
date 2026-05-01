package store

import (
	"database/sql"
	"strings"
	"time"
)

type ConnectorCommand struct {
	ID          int64     `json:"id"`
	OrgID       int64     `json:"org_id"`
	AccountID   int64     `json:"account_id"`
	AgentID     int64     `json:"agent_id"`
	Type        string    `json:"type"`
	PayloadJSON string    `json:"payload_json"`
	Status      string    `json:"status"`
	ErrorMsg    string    `json:"error_msg"`
	CreatedBy   int64     `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
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

package sessions

import (
	"context"
	"database/sql"
	"time"
)

// BrowserSession tracks the lifecycle of one account's Chrome session.
type BrowserSession struct {
	ID           int64     `json:"id"`
	AccountID    int64     `json:"account_id"`
	OrgID        int64     `json:"org_id"`
	Status       string    `json:"status"` // active|idle|error|terminated
	CDPPort      int       `json:"cdp_port"`
	VNCPort      int       `json:"vnc_port,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	ErrorMsg     string    `json:"error_msg,omitempty"`
}

func (s *Store) UpsertSession(ctx context.Context, sess BrowserSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO browser_sessions
			(account_id, org_id, status, cdp_port, vnc_port, started_at, last_active_at, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			status        = excluded.status,
			cdp_port      = excluded.cdp_port,
			vnc_port      = excluded.vnc_port,
			last_active_at= excluded.last_active_at,
			error_msg     = excluded.error_msg`,
		sess.AccountID, sess.OrgID, sess.Status, sess.CDPPort, sess.VNCPort,
		sess.StartedAt.UTC(), sess.LastActiveAt.UTC(), sess.ErrorMsg,
	)
	return err
}

func (s *Store) GetSession(ctx context.Context, accountID int64) (*BrowserSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, org_id, status, cdp_port, vnc_port,
		       started_at, last_active_at, error_msg
		FROM browser_sessions WHERE account_id = ?`, accountID)
	return scanSession(row)
}

// ListAllActiveSessions returns all sessions that are not terminated.
// Used by the in-memory Registry to seed its state on startup.
func (s *Store) ListAllActiveSessions(ctx context.Context) ([]BrowserSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, org_id, status, cdp_port, vnc_port,
		       started_at, last_active_at, error_msg
		FROM browser_sessions WHERE status != 'terminated'
		ORDER BY last_active_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BrowserSession
	for rows.Next() {
		bs, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *bs)
	}
	return out, rows.Err()
}

func (s *Store) ListSessions(ctx context.Context, orgID int64) ([]BrowserSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, org_id, status, cdp_port, vnc_port,
		       started_at, last_active_at, error_msg
		FROM browser_sessions WHERE org_id = ?
		ORDER BY last_active_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BrowserSession
	for rows.Next() {
		bs, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *bs)
	}
	return out, rows.Err()
}

// GetFirstActiveCDPSession returns the first non-terminated browser session
// with a reachable CDP port. Legacy callers use this when they do not own the
// allocator path yet.
func (s *Store) GetFirstActiveCDPSession(ctx context.Context) (*BrowserSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, org_id, status, cdp_port, vnc_port,
		       started_at, last_active_at, error_msg
		FROM browser_sessions
		WHERE status IN ('idle', 'ready', 'active') AND cdp_port > 0
		ORDER BY last_active_at DESC
		LIMIT 1`)
	return scanSession(row)
}

func (s *Store) TerminateSession(ctx context.Context, accountID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE browser_sessions SET status='terminated', last_active_at=? WHERE account_id=?`,
		time.Now().UTC(), accountID)
	return err
}

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSession(r sessionScanner) (*BrowserSession, error) {
	var s BrowserSession
	var startedAt, lastActiveAt string
	err := r.Scan(&s.ID, &s.AccountID, &s.OrgID, &s.Status, &s.CDPPort, &s.VNCPort,
		&startedAt, &lastActiveAt, &s.ErrorMsg)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
	s.LastActiveAt, _ = time.Parse(time.RFC3339Nano, lastActiveAt)
	return &s, nil
}

// Domain: app (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"time"
)

// BrowserIdentity stores the persisted fingerprint + session-state metadata for an account.
type BrowserIdentity struct {
	ID            int64     `json:"id"`
	AccountID     int64     `json:"account_id"`
	OrgID         int64     `json:"org_id"`
	UserAgent     string    `json:"user_agent"`
	ScreenW       int       `json:"screen_w"`
	ScreenH       int       `json:"screen_h"`
	Timezone      string    `json:"timezone"`
	Languages     string    `json:"languages"`
	WebGLVendor   string    `json:"webgl_vendor"`
	WebGLRenderer string    `json:"webgl_renderer"`
	SessionState  string    `json:"session_state"` // clean|warned|restricted|banned
	UpdatedAt     time.Time `json:"updated_at"`
}

func (a *AppStore) UpsertIdentity(ctx context.Context, bi BrowserIdentity) error {
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO browser_identities
			(account_id, org_id, user_agent, screen_w, screen_h, timezone,
			 languages, webgl_vendor, webgl_renderer, session_state, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			user_agent     = excluded.user_agent,
			screen_w       = excluded.screen_w,
			screen_h       = excluded.screen_h,
			timezone       = excluded.timezone,
			languages      = excluded.languages,
			webgl_vendor   = excluded.webgl_vendor,
			webgl_renderer = excluded.webgl_renderer,
			session_state  = excluded.session_state,
			updated_at     = excluded.updated_at`,
		bi.AccountID, bi.OrgID, bi.UserAgent, bi.ScreenW, bi.ScreenH, bi.Timezone,
		bi.Languages, bi.WebGLVendor, bi.WebGLRenderer, bi.SessionState,
		bi.UpdatedAt.UTC(),
	)
	return err
}

func (a *AppStore) GetIdentity(ctx context.Context, accountID int64) (*BrowserIdentity, error) {
	row := a.db.QueryRowContext(ctx, `
		SELECT id, account_id, org_id, user_agent, screen_w, screen_h, timezone,
		       languages, webgl_vendor, webgl_renderer, session_state, updated_at
		FROM browser_identities WHERE account_id = ?`, accountID)
	return scanIdentity(row)
}

func (a *AppStore) ListIdentities(ctx context.Context, orgID int64) ([]BrowserIdentity, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, account_id, org_id, user_agent, screen_w, screen_h, timezone,
		       languages, webgl_vendor, webgl_renderer, session_state, updated_at
		FROM browser_identities WHERE org_id = ?
		ORDER BY updated_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BrowserIdentity
	for rows.Next() {
		bi, err := scanIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *bi)
	}
	return out, rows.Err()
}

func (a *AppStore) SetSessionState(ctx context.Context, accountID int64, state string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE browser_identities SET session_state=?, updated_at=? WHERE account_id=?`,
		state, time.Now().UTC(), accountID)
	return err
}

type identityScanner interface {
	Scan(dest ...any) error
}

func scanIdentity(r identityScanner) (*BrowserIdentity, error) {
	var bi BrowserIdentity
	var updatedAt string
	err := r.Scan(
		&bi.ID, &bi.AccountID, &bi.OrgID, &bi.UserAgent, &bi.ScreenW, &bi.ScreenH,
		&bi.Timezone, &bi.Languages, &bi.WebGLVendor, &bi.WebGLRenderer,
		&bi.SessionState, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	bi.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &bi, nil
}

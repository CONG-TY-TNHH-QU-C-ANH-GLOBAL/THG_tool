package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// AgentToken mirrors the agent_tokens table.
type AgentToken struct {
	ID                int64      `json:"id"`
	OrgID             int64      `json:"org_id"`
	Name              string     `json:"name"`
	CreatedBy         int64      `json:"created_by"`
	Hostname          string     `json:"hostname"`
	OS                string     `json:"os"`
	Version           string     `json:"version"`
	Kind              string     `json:"kind"`
	Transport         string     `json:"transport"`
	AssignedAccountID int64      `json:"assigned_account_id"`
	CapabilitiesJSON  string     `json:"capabilities_json"`
	CurrentURL        string     `json:"current_url"`
	FBUserID          string     `json:"fb_user_id"`
	StreamStatus      string     `json:"stream_status"`
	LastSeen          *time.Time `json:"last_seen"`
	Online            bool       `json:"online"`
	Active            bool       `json:"active"`
	CreatedAt         time.Time  `json:"created_at"`
}

type AgentPresence struct {
	Hostname          string
	OS                string
	Version           string
	Kind              string
	Transport         string
	AssignedAccountID int64
	CapabilitiesJSON  string
	CurrentURL        string
	FBUserID          string
	StreamStatus      string
}

type ConnectorPairingCode struct {
	ID                int64
	OrgID             int64
	Name              string
	Code              string
	CreatedBy         int64
	AssignedAccountID int64
	ExpiresAt         time.Time
	CreatedAt         time.Time
}

type ConnectorScreenshot struct {
	AccountID    int64     `json:"account_id"`
	OrgID        int64     `json:"org_id"`
	AgentID      int64     `json:"agent_id"`
	ImageData    string    `json:"image_data"`
	CurrentURL   string    `json:"current_url"`
	FBUserID     string    `json:"fb_user_id"`
	StreamStatus string    `json:"stream_status"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LocalBrowserTarget struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	FBUserID    string `json:"fb_user_id"`
	Status      string `json:"status"`
}

func hashAgentToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

func hashPairingCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	normalized := b.String()
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// CreateAgentToken generates a new token, stores its hash, and returns the plaintext (shown once).
func (s *Store) CreateAgentToken(name string, createdBy, orgID int64) (int64, string, error) {
	return s.createAgentToken(name, createdBy, orgID, "worker", 0)
}

func (s *Store) createAgentToken(name string, createdBy, orgID int64, kind string, accountID int64) (int64, string, error) {
	transport := "poll"
	if kind == "desktop_connector" || kind == "extension_connector" {
		transport = "websocket"
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return 0, "", fmt.Errorf("generate token: %w", err)
	}
	plain := hex.EncodeToString(b)
	hash := hashAgentToken(plain)

	res, err := s.db.Exec(
		`INSERT INTO agent_tokens (org_id, name, token_hash, created_by, kind, transport, assigned_account_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		orgID, name, hash, createdBy, kind, transport, accountID,
	)
	if err != nil {
		return 0, "", err
	}
	id, _ := res.LastInsertId()
	return id, plain, nil
}

func generatePairingCode(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var sb strings.Builder
	for sb.Len() < length {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		sb.WriteByte(alphabet[n.Int64()])
	}
	code := sb.String()
	if len(code) == 8 {
		return code[:4] + "-" + code[4:], nil
	}
	return code, nil
}

func (s *Store) CreateConnectorPairingCode(name string, createdBy, orgID, accountID int64, ttl time.Duration) (*ConnectorPairingCode, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if name == "" {
		name = "Local Chrome"
	}
	expiresAt := time.Now().Add(ttl).UTC()
	_, _ = s.db.Exec(
		`UPDATE connector_pairing_codes
		 SET used_at = CURRENT_TIMESTAMP
		 WHERE org_id = ? AND created_by = ? AND assigned_account_id = ? AND used_at IS NULL`,
		orgID, createdBy, accountID,
	)
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generatePairingCode(8)
		if err != nil {
			return nil, fmt.Errorf("generate pairing code: %w", err)
		}
		res, err := s.db.Exec(
			`INSERT INTO connector_pairing_codes (org_id, code_hash, name, created_by, assigned_account_id, expires_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			orgID, hashPairingCode(code), name, createdBy, accountID, expiresAt,
		)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			return nil, err
		}
		id, _ := res.LastInsertId()
		return &ConnectorPairingCode{
			ID:                id,
			OrgID:             orgID,
			Name:              name,
			Code:              code,
			CreatedBy:         createdBy,
			AssignedAccountID: accountID,
			ExpiresAt:         expiresAt,
		}, nil
	}
	return nil, fmt.Errorf("could not allocate unique pairing code")
}

func (s *Store) ClaimConnectorPairingCode(code string, p AgentPresence) (*AgentToken, string, error) {
	hash := hashPairingCode(code)
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback() //nolint:errcheck

	var row struct {
		ID        int64
		OrgID     int64
		Name      string
		CreatedBy int64
		AccountID int64
		ExpiresAt time.Time
		UsedAt    sql.NullTime
	}
	err = tx.QueryRow(
		`SELECT id, org_id, name, created_by, assigned_account_id, expires_at, used_at
		 FROM connector_pairing_codes
		 WHERE code_hash = ?`,
		hash,
	).Scan(&row.ID, &row.OrgID, &row.Name, &row.CreatedBy, &row.AccountID, &row.ExpiresAt, &row.UsedAt)
	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("invalid pairing code")
	}
	if err != nil {
		return nil, "", err
	}
	if row.UsedAt.Valid {
		return nil, "", fmt.Errorf("pairing code already used")
	}
	if time.Now().UTC().After(row.ExpiresAt.UTC()) {
		return nil, "", fmt.Errorf("pairing code expired")
	}

	deviceName := strings.TrimSpace(row.Name)
	if p.Hostname != "" {
		deviceName = fmt.Sprintf("%s - %s", row.Name, p.Hostname)
	}
	if deviceName == "" {
		deviceName = "Local Chrome"
	}
	kind := strings.TrimSpace(p.Kind)
	if kind == "" {
		kind = "desktop_connector"
	}
	if kind != "desktop_connector" && kind != "extension_connector" {
		kind = "desktop_connector"
	}
	transport := strings.TrimSpace(p.Transport)
	if transport == "" {
		if kind == "extension_connector" {
			transport = "chrome_extension"
		} else {
			transport = "local_chrome"
		}
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, "", fmt.Errorf("generate token: %w", err)
	}
	plain := hex.EncodeToString(b)
	tokenHash := hashAgentToken(plain)

	res, err := tx.Exec(
		`INSERT INTO agent_tokens (
			org_id, name, token_hash, created_by, kind, transport, assigned_account_id,
			hostname, os, version, capabilities_json, current_url, fb_user_id, stream_status, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		row.OrgID, deviceName, tokenHash, row.CreatedBy, kind, transport, row.AccountID,
		p.Hostname, p.OS, p.Version, defaultString(p.CapabilitiesJSON, "{}"), p.CurrentURL, p.FBUserID, defaultString(p.StreamStatus, "idle"),
	)
	if err != nil {
		return nil, "", err
	}
	tokenID, _ := res.LastInsertId()
	updateRes, err := tx.Exec(
		`UPDATE connector_pairing_codes SET used_at = CURRENT_TIMESTAMP, device_token_id = ? WHERE id = ? AND used_at IS NULL`,
		tokenID, row.ID,
	)
	if err != nil {
		return nil, "", err
	}
	if affected, err := updateRes.RowsAffected(); err == nil && affected != 1 {
		return nil, "", fmt.Errorf("pairing code already claimed")
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	tok, err := s.ValidateAgentToken(plain)
	if err != nil {
		return nil, "", err
	}
	return tok, plain, nil
}

// ValidateAgentToken checks the token hash and returns the token record if valid.
func (s *Store) ValidateAgentToken(plain string) (*AgentToken, error) {
	hash := hashAgentToken(plain)
	row := s.db.QueryRow(
		`SELECT id, COALESCE(org_id,0), name, created_by,
		        COALESCE(hostname,''), COALESCE(os,''), COALESCE(version,''),
		        COALESCE(kind,'worker'), COALESCE(transport,'poll'), COALESCE(assigned_account_id,0),
		        COALESCE(capabilities_json,'{}'), COALESCE(current_url,''), COALESCE(fb_user_id,''), COALESCE(stream_status,'idle'),
		        last_seen, active, created_at
		 FROM agent_tokens WHERE token_hash = ? AND active = 1`,
		hash,
	)
	var t AgentToken
	var lastSeen sql.NullTime
	err := row.Scan(&t.ID, &t.OrgID, &t.Name, &t.CreatedBy, &t.Hostname, &t.OS, &t.Version,
		&t.Kind, &t.Transport, &t.AssignedAccountID, &t.CapabilitiesJSON, &t.CurrentURL, &t.FBUserID, &t.StreamStatus,
		&lastSeen, &t.Active, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastSeen.Valid {
		t.LastSeen = &lastSeen.Time
	}
	t.Online = agentOnline(t.LastSeen, t.Active)
	return &t, nil
}

// UpdateAgentHeartbeat records agent liveness and machine info.
func (s *Store) UpdateAgentHeartbeat(id int64, hostname, os, version string) error {
	return s.UpdateAgentPresence(id, AgentPresence{Hostname: hostname, OS: os, Version: version})
}

func (s *Store) UpdateAgentPresence(id int64, p AgentPresence) error {
	_, err := s.db.Exec(
		`UPDATE agent_tokens SET
			hostname = CASE WHEN ? != '' THEN ? ELSE hostname END,
			os = CASE WHEN ? != '' THEN ? ELSE os END,
			version = CASE WHEN ? != '' THEN ? ELSE version END,
			kind = CASE WHEN ? != '' THEN ? ELSE kind END,
			transport = CASE WHEN ? != '' THEN ? ELSE transport END,
			assigned_account_id = CASE WHEN ? > 0 THEN ? ELSE assigned_account_id END,
			capabilities_json = CASE WHEN ? != '' THEN ? ELSE capabilities_json END,
			current_url = CASE WHEN ? != '' THEN ? ELSE current_url END,
			fb_user_id = CASE WHEN ? != '' THEN ? ELSE fb_user_id END,
			stream_status = CASE WHEN ? != '' THEN ? ELSE stream_status END,
			last_seen = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		p.Hostname, p.Hostname,
		p.OS, p.OS,
		p.Version, p.Version,
		p.Kind, p.Kind,
		p.Transport, p.Transport,
		p.AssignedAccountID, p.AssignedAccountID,
		p.CapabilitiesJSON, p.CapabilitiesJSON,
		p.CurrentURL, p.CurrentURL,
		p.FBUserID, p.FBUserID,
		p.StreamStatus, p.StreamStatus,
		id,
	)
	return err
}

// ListAgentTokens returns all tokens for the admin UI (no token hashes exposed).
func (s *Store) ListAgentTokens(orgID int64) ([]AgentToken, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(org_id,0), name, created_by,
		        COALESCE(hostname,''), COALESCE(os,''), COALESCE(version,''),
		        COALESCE(kind,'worker'), COALESCE(transport,'poll'), COALESCE(assigned_account_id,0),
		        COALESCE(capabilities_json,'{}'), COALESCE(current_url,''), COALESCE(fb_user_id,''), COALESCE(stream_status,'idle'),
		        last_seen, active, created_at
		 FROM agent_tokens WHERE org_id = ? ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentToken
	for rows.Next() {
		var t AgentToken
		var lastSeen sql.NullTime
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.CreatedBy, &t.Hostname, &t.OS, &t.Version,
			&t.Kind, &t.Transport, &t.AssignedAccountID, &t.CapabilitiesJSON, &t.CurrentURL, &t.FBUserID, &t.StreamStatus,
			&lastSeen, &t.Active, &t.CreatedAt); err != nil {
			return nil, err
		}
		if lastSeen.Valid {
			t.LastSeen = &lastSeen.Time
		}
		t.Online = agentOnline(t.LastSeen, t.Active)
		out = append(out, t)
	}
	return out, nil
}

func (s *Store) ListLocalConnectors(orgID int64) ([]AgentToken, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(org_id,0), name, created_by,
		        COALESCE(hostname,''), COALESCE(os,''), COALESCE(version,''),
		        COALESCE(kind,'worker'), COALESCE(transport,'poll'), COALESCE(assigned_account_id,0),
		        COALESCE(capabilities_json,'{}'), COALESCE(current_url,''), COALESCE(fb_user_id,''), COALESCE(stream_status,'idle'),
		        last_seen, active, created_at
		 FROM agent_tokens
		 WHERE org_id = ? AND kind IN ('desktop_connector', 'extension_connector')
		   AND active = 1
		 ORDER BY last_seen DESC, created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentToken
	for rows.Next() {
		var t AgentToken
		var lastSeen sql.NullTime
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.CreatedBy, &t.Hostname, &t.OS, &t.Version,
			&t.Kind, &t.Transport, &t.AssignedAccountID, &t.CapabilitiesJSON, &t.CurrentURL, &t.FBUserID, &t.StreamStatus,
			&lastSeen, &t.Active, &t.CreatedAt); err != nil {
			return nil, err
		}
		if lastSeen.Valid {
			t.LastSeen = &lastSeen.Time
		}
		t.Online = agentOnline(t.LastSeen, t.Active)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) AssignAgentAccount(id, orgID, accountID int64) error {
	res, err := s.db.Exec(
		`UPDATE agent_tokens SET assigned_account_id = ? WHERE id = ? AND org_id = ? AND active = 1`,
		accountID, id, orgID,
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

func (s *Store) UpsertConnectorScreenshot(agentID, orgID, accountID int64, imageData, currentURL, fbUserID, streamStatus string) error {
	if accountID <= 0 {
		accountID = 0
	}
	_, err := s.db.Exec(
		`INSERT INTO connector_screenshots
			(account_id, org_id, agent_id, image_data, current_url, fb_user_id, stream_status, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(org_id, account_id) DO UPDATE SET
			agent_id = excluded.agent_id,
			image_data = excluded.image_data,
			current_url = excluded.current_url,
			fb_user_id = excluded.fb_user_id,
			stream_status = excluded.stream_status,
			updated_at = CURRENT_TIMESTAMP`,
		accountID, orgID, agentID, imageData, currentURL, fbUserID, streamStatus,
	)
	return err
}

func (s *Store) GetLatestConnectorScreenshot(orgID, accountID int64) (*ConnectorScreenshot, error) {
	query := `SELECT account_id, org_id, agent_id, image_data, current_url, fb_user_id, stream_status, updated_at
		FROM connector_screenshots WHERE org_id = ?`
	args := []any{orgID}
	if accountID > 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY updated_at DESC LIMIT 1`

	var out ConnectorScreenshot
	var updatedAt string
	err := s.db.QueryRow(query, args...).Scan(
		&out.AccountID, &out.OrgID, &out.AgentID, &out.ImageData, &out.CurrentURL, &out.FBUserID, &out.StreamStatus, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if parsed, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		out.UpdatedAt = parsed
	} else if parsed, err := time.Parse("2006-01-02 15:04:05", updatedAt); err == nil {
		out.UpdatedAt = parsed
	}
	return &out, nil
}

func (s *Store) StopLocalSessionsForConnector(agentID, orgID int64) error {
	_, err := s.db.Exec(
		`UPDATE browser_sessions
		 SET status = 'local_stopped',
		     last_active_at = CURRENT_TIMESTAMP,
		     error_msg = ''
		 WHERE org_id = ?
		   AND status LIKE 'local_%'
		   AND account_id IN (
		   	SELECT account_id FROM connector_screenshots WHERE agent_id = ? AND org_id = ?
		   )`,
		orgID, agentID, orgID,
	)
	return err
}

func (s *Store) StopAllLocalSessionsForOrg(orgID int64) error {
	_, err := s.db.Exec(
		`UPDATE browser_sessions
		 SET status = 'local_stopped',
		     last_active_at = CURRENT_TIMESTAMP,
		     error_msg = ''
		 WHERE org_id = ?
		   AND status LIKE 'local_%'`,
		orgID,
	)
	return err
}

func (s *Store) DeleteConnectorScreenshotsByAgent(agentID, orgID int64) error {
	_, err := s.db.Exec(`DELETE FROM connector_screenshots WHERE agent_id = ? AND org_id = ?`, agentID, orgID)
	return err
}

func (s *Store) ListLocalBrowserTargets(orgID int64) ([]LocalBrowserTarget, error) {
	rows, err := s.db.Query(
		`SELECT a.id, a.name, COALESCE(a.fb_user_id,''), COALESCE(bs.status,'local_starting')
		 FROM accounts a
		 JOIN browser_sessions bs ON bs.account_id = a.id
		 WHERE a.org_id = ?
		   AND a.platform = 'facebook'
		   AND bs.status LIKE 'local_%'
		   AND bs.status != 'local_stopped'
		   AND bs.status != 'terminated'
		 ORDER BY bs.last_active_at DESC, a.id DESC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LocalBrowserTarget
	for rows.Next() {
		var t LocalBrowserTarget
		if err := rows.Scan(&t.AccountID, &t.AccountName, &t.FBUserID, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeAgentToken deactivates a token by ID.
func (s *Store) RevokeAgentToken(id, orgID int64) error {
	res, err := s.db.Exec(`UPDATE agent_tokens SET active = 0 WHERE id = ? AND org_id = ?`, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func agentOnline(lastSeen *time.Time, active bool) bool {
	return active && lastSeen != nil && time.Since(*lastSeen) <= 90*time.Second
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// InsertPostsBatch inserts a slice of posts from an agent result, skipping duplicates.
func (s *Store) InsertPostsBatch(posts []models.Post) (int, error) {
	saved := 0
	for i := range posts {
		p := &posts[i]
		if p.DedupHash == "" {
			h := sha256.Sum256([]byte(p.Content + p.AuthorURL))
			p.DedupHash = hex.EncodeToString(h[:])
		}
		_, err := s.db.Exec(
			`INSERT OR IGNORE INTO posts (platform, group_id, group_name, url, author, author_url, content, images, reactions, comments, posted_at, scraped_at, dedup_hash)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
			p.Platform, p.GroupID, p.GroupName, p.URL, p.Author, p.AuthorURL,
			p.Content, p.Images, p.Reactions, p.Comments, p.PostedAt, p.DedupHash,
		)
		if err == nil {
			saved++
		}
	}
	return saved, nil
}

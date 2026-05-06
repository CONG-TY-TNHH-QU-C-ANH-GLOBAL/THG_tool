package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/browsergateway"
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
	FBDisplayName     string     `json:"fb_display_name"`
	FBUsername        string     `json:"fb_username"`
	FBProfileURL      string     `json:"fb_profile_url"`
	StreamStatus      string     `json:"stream_status"`
	ChromeError       string     `json:"chrome_error"`
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
	FBDisplayName     string
	FBUsername        string
	FBProfileURL      string
	StreamStatus      string
	ChromeError       string
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
	AccountID     int64     `json:"account_id"`
	OrgID         int64     `json:"org_id"`
	AgentID       int64     `json:"agent_id"`
	ImageData     string    `json:"image_data"`
	CurrentURL    string    `json:"current_url"`
	FBUserID      string    `json:"fb_user_id"`
	FBDisplayName string    `json:"fb_display_name"`
	FBUsername    string    `json:"fb_username"`
	FBProfileURL  string    `json:"fb_profile_url"`
	StreamStatus  string    `json:"stream_status"`
	ChromeError   string    `json:"chrome_error"`
	UpdatedAt     time.Time `json:"updated_at"`
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
	if kind == browsergateway.KindExtensionConnector {
		transport = browsergateway.TransportChromeExtension
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

// ValidateAgentToken checks the token hash and returns the token record if valid.
func (s *Store) ValidateAgentToken(plain string) (*AgentToken, error) {
	hash := hashAgentToken(plain)
	row := s.db.QueryRow(
		`SELECT id, COALESCE(org_id,0), name, created_by,
		        COALESCE(hostname,''), COALESCE(os,''), COALESCE(version,''),
		        COALESCE(kind,'worker'), COALESCE(transport,'poll'), COALESCE(assigned_account_id,0),
		        COALESCE(capabilities_json,'{}'), COALESCE(current_url,''), COALESCE(fb_user_id,''),
		        COALESCE(fb_display_name,''), COALESCE(fb_username,''), COALESCE(fb_profile_url,''),
		        COALESCE(stream_status,'idle'), COALESCE(chrome_error,''),
		        last_seen, active, created_at
		 FROM agent_tokens WHERE token_hash = ? AND active = 1`,
		hash,
	)
	var t AgentToken
	var lastSeen sql.NullTime
	err := row.Scan(&t.ID, &t.OrgID, &t.Name, &t.CreatedBy, &t.Hostname, &t.OS, &t.Version,
		&t.Kind, &t.Transport, &t.AssignedAccountID, &t.CapabilitiesJSON, &t.CurrentURL, &t.FBUserID,
		&t.FBDisplayName, &t.FBUsername, &t.FBProfileURL, &t.StreamStatus, &t.ChromeError,
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
			fb_display_name = CASE WHEN ? != '' THEN ? ELSE fb_display_name END,
			fb_username = CASE WHEN ? != '' THEN ? ELSE fb_username END,
			fb_profile_url = CASE WHEN ? != '' THEN ? ELSE fb_profile_url END,
			stream_status = CASE WHEN ? != '' THEN ? ELSE stream_status END,
			chrome_error = ?,
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
		p.FBDisplayName, p.FBDisplayName,
		p.FBUsername, p.FBUsername,
		p.FBProfileURL, p.FBProfileURL,
		p.StreamStatus, p.StreamStatus,
		p.ChromeError,
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
		        COALESCE(capabilities_json,'{}'), COALESCE(current_url,''), COALESCE(fb_user_id,''),
		        COALESCE(fb_display_name,''), COALESCE(fb_username,''), COALESCE(fb_profile_url,''),
		        COALESCE(stream_status,'idle'), COALESCE(chrome_error,''),
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
			&t.Kind, &t.Transport, &t.AssignedAccountID, &t.CapabilitiesJSON, &t.CurrentURL, &t.FBUserID,
			&t.FBDisplayName, &t.FBUsername, &t.FBProfileURL, &t.StreamStatus, &t.ChromeError,
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
		        COALESCE(capabilities_json,'{}'), COALESCE(current_url,''), COALESCE(fb_user_id,''),
		        COALESCE(fb_display_name,''), COALESCE(fb_username,''), COALESCE(fb_profile_url,''),
		        COALESCE(stream_status,'idle'), COALESCE(chrome_error,''),
		        last_seen, active, created_at
		 FROM agent_tokens
		 WHERE org_id = ? AND kind = 'extension_connector'
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
			&t.Kind, &t.Transport, &t.AssignedAccountID, &t.CapabilitiesJSON, &t.CurrentURL, &t.FBUserID,
			&t.FBDisplayName, &t.FBUsername, &t.FBProfileURL, &t.StreamStatus, &t.ChromeError,
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

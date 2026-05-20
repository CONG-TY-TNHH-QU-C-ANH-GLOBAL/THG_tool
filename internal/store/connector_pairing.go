// Domain: connectors (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/thg/scraper/internal/browsergateway"
)

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
		name = browsergateway.DefaultChromeConnectorName
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
		deviceName = browsergateway.DefaultChromeConnectorName
	}
	kind := strings.TrimSpace(p.Kind)
	if kind == "" {
		kind = browsergateway.KindExtensionConnector
	}
	if kind != browsergateway.KindExtensionConnector {
		kind = browsergateway.KindExtensionConnector
	}
	transport := strings.TrimSpace(p.Transport)
	if transport == "" {
		transport = browsergateway.TransportChromeExtension
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
			hostname, os, version, capabilities_json, current_url, fb_user_id,
			fb_display_name, fb_username, fb_profile_url, stream_status, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		row.OrgID, deviceName, tokenHash, row.CreatedBy, kind, transport, row.AccountID,
		p.Hostname, p.OS, p.Version, defaultString(p.CapabilitiesJSON, "{}"), p.CurrentURL, p.FBUserID,
		p.FBDisplayName, p.FBUsername, p.FBProfileURL, defaultString(p.StreamStatus, "idle"),
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

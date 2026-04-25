package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/thg/scraper/internal/models"
)

// AgentToken mirrors the agent_tokens table.
type AgentToken struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	CreatedBy int64      `json:"created_by"`
	Hostname  string     `json:"hostname"`
	OS        string     `json:"os"`
	Version   string     `json:"version"`
	LastSeen  *time.Time `json:"last_seen"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
}

func hashAgentToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// CreateAgentToken generates a new token, stores its hash, and returns the plaintext (shown once).
func (s *Store) CreateAgentToken(name string, createdBy int64) (int64, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return 0, "", fmt.Errorf("generate token: %w", err)
	}
	plain := hex.EncodeToString(b)
	hash := hashAgentToken(plain)

	res, err := s.db.Exec(
		`INSERT INTO agent_tokens (name, token_hash, created_by) VALUES (?, ?, ?)`,
		name, hash, createdBy,
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
		`SELECT id, name, created_by, COALESCE(hostname,''), COALESCE(os,''), COALESCE(version,''), last_seen, active, created_at FROM agent_tokens WHERE token_hash = ? AND active = 1`,
		hash,
	)
	var t AgentToken
	var lastSeen sql.NullTime
	err := row.Scan(&t.ID, &t.Name, &t.CreatedBy, &t.Hostname, &t.OS, &t.Version, &lastSeen, &t.Active, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastSeen.Valid {
		t.LastSeen = &lastSeen.Time
	}
	return &t, nil
}

// UpdateAgentHeartbeat records agent liveness and machine info.
func (s *Store) UpdateAgentHeartbeat(id int64, hostname, os, version string) error {
	_, err := s.db.Exec(
		`UPDATE agent_tokens SET hostname = ?, os = ?, version = ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?`,
		hostname, os, version, id,
	)
	return err
}

// ListAgentTokens returns all tokens for the admin UI (no token hashes exposed).
func (s *Store) ListAgentTokens() ([]AgentToken, error) {
	rows, err := s.db.Query(
		`SELECT id, name, created_by, COALESCE(hostname,''), COALESCE(os,''), COALESCE(version,''), last_seen, active, created_at FROM agent_tokens ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentToken
	for rows.Next() {
		var t AgentToken
		var lastSeen sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedBy, &t.Hostname, &t.OS, &t.Version, &lastSeen, &t.Active, &t.CreatedAt); err != nil {
			return nil, err
		}
		if lastSeen.Valid {
			t.LastSeen = &lastSeen.Time
		}
		out = append(out, t)
	}
	return out, nil
}

// RevokeAgentToken deactivates a token by ID.
func (s *Store) RevokeAgentToken(id int64) error {
	_, err := s.db.Exec(`UPDATE agent_tokens SET active = 0 WHERE id = ?`, id)
	return err
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

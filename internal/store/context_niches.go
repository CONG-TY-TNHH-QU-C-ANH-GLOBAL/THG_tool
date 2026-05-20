// Domain: leads (see internal/store/DOMAINS.md)
package store

import "github.com/thg/scraper/internal/models"

// SetContext stores a key-value configuration.
func (s *Store) SetContext(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO user_context (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

// GetContext retrieves a context value by key.
func (s *Store) GetContext(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM user_context WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

// GetAllContext returns all stored context.
func (s *Store) GetAllContext() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM user_context ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ctx := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			ctx[k] = v
		}
	}
	return ctx, nil
}

// GetNiches returns all niches.
func (s *Store) GetNiches() ([]models.Niche, error) {
	rows, err := s.db.Query(`SELECT id, slug, name, emoji, active, created_at FROM niches ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var niches []models.Niche
	for rows.Next() {
		var n models.Niche
		var active int
		if err := rows.Scan(&n.ID, &n.Slug, &n.Name, &n.Emoji, &active, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.Active = active == 1
		niches = append(niches, n)
	}
	return niches, nil
}

// InsertNiche adds a new niche.
func (s *Store) InsertNiche(n *models.Niche) (int64, error) {
	if n.Emoji == "" {
		n.Emoji = "target"
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO niches (slug, name, emoji) VALUES (?, ?, ?)`,
		n.Slug, n.Name, n.Emoji,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteNiche removes a niche by slug.
func (s *Store) DeleteNiche(slug string) error {
	_, err := s.db.Exec(`DELETE FROM niches WHERE slug = ?`, slug)
	return err
}

// HasSentComment / HasContactedCandidate / HasSentInbox /
// DeleteAllOutboundComments were removed in PR-2 (V2 tenant isolation).
// All four were non-org-scoped reads/writes of outbound_messages —
// cross-tenant leaks — and audit found ZERO callers anywhere in the
// codebase. Use the ForOrg dedup gate via Store.CheckDedupTx /
// canQueueOutboundTx instead.

// DeleteAllOutboundCommentsForOrg deletes comment outbox rows only for one tenant.
func (s *Store) DeleteAllOutboundCommentsForOrg(orgID int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM outbound_messages WHERE type = 'comment' AND org_id = ?`, orgID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeleteAllOutboundPostsForOrg deletes posting outbox rows (group + profile
// posts) for one tenant. The Posting dashboard view shows exactly these two
// types, so "delete all posting" maps to this set.
func (s *Store) DeleteAllOutboundPostsForOrg(orgID int64) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM outbound_messages WHERE type IN ('group_post','profile_post') AND org_id = ?`,
		orgID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}


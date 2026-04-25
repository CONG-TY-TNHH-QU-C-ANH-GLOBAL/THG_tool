package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// SelectorCache stores working CSS/XPath selectors per action+platform.
// Updated automatically when LLM Vision discovers new working selectors.
type SelectorCache struct {
	ID        int64
	Action    string
	Platform  string
	Selectors map[string]string
	HitCount  int
	UpdatedAt time.Time
}

func (s *Store) initSelectorCache() {
	s.db.Exec(`CREATE TABLE IF NOT EXISTS selector_cache (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		action      TEXT NOT NULL,
		platform    TEXT NOT NULL,
		selectors   TEXT NOT NULL DEFAULT '{}',
		hit_count   INTEGER NOT NULL DEFAULT 0,
		updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(action, platform)
	)`)
}

// GetSelectors returns cached selectors for action+platform, or nil if not found.
func (s *Store) GetSelectors(action, platform string) (map[string]string, error) {
	var raw string
	err := s.db.QueryRow(
		`SELECT selectors FROM selector_cache WHERE action=? AND platform=?`,
		action, platform,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	// bump hit counter asynchronously
	go s.db.Exec(`UPDATE selector_cache SET hit_count=hit_count+1 WHERE action=? AND platform=?`, action, platform)
	return m, nil
}

// UpsertSelectors saves (or replaces) selectors for action+platform.
// Called after LLM Vision discovers new working selectors.
func (s *Store) UpsertSelectors(action, platform string, selectors map[string]string) error {
	raw, err := json.Marshal(selectors)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO selector_cache (action, platform, selectors, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(action, platform) DO UPDATE SET
			selectors  = excluded.selectors,
			hit_count  = 0,
			updated_at = CURRENT_TIMESTAMP
	`, action, platform, string(raw))
	return err
}

// ListSelectorCache returns all cached entries (for admin inspection).
func (s *Store) ListSelectorCache() ([]*SelectorCache, error) {
	rows, err := s.db.Query(`SELECT id, action, platform, selectors, hit_count, updated_at FROM selector_cache ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*SelectorCache
	for rows.Next() {
		sc := &SelectorCache{}
		var raw string
		if err := rows.Scan(&sc.ID, &sc.Action, &sc.Platform, &raw, &sc.HitCount, &sc.UpdatedAt); err != nil {
			continue
		}
		json.Unmarshal([]byte(raw), &sc.Selectors)
		out = append(out, sc)
	}
	return out, nil
}

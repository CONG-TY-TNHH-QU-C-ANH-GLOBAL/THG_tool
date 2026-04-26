package runtime

import (
	"context"
	"database/sql"
	"time"
)

// PaginationCache deduplicates posts across job runs for the same source URL.
// Prevents re-processing posts already seen in previous scrape cycles.
type PaginationCache interface {
	Seen(sourceURL, postID string) bool
	Mark(sourceURL, postID string)
	Flush(ctx context.Context, sourceURL string) error
	PruneOlderThan(ctx context.Context, age time.Duration) error
}

// SQLitePaginationCache stores seen post IDs in the post_seen_cache table.
type SQLitePaginationCache struct {
	db *sql.DB
}

// NewSQLitePaginationCache creates a cache backed by the given DB connection.
func NewSQLitePaginationCache(db *sql.DB) *SQLitePaginationCache {
	return &SQLitePaginationCache{db: db}
}

func (c *SQLitePaginationCache) Seen(sourceURL, postID string) bool {
	var exists int
	err := c.db.QueryRow(
		`SELECT 1 FROM post_seen_cache WHERE source_url=? AND post_id=? LIMIT 1`,
		sourceURL, postID,
	).Scan(&exists)
	return err == nil && exists == 1
}

func (c *SQLitePaginationCache) Mark(sourceURL, postID string) {
	c.db.Exec(
		`INSERT OR IGNORE INTO post_seen_cache (source_url, post_id) VALUES (?, ?)`,
		sourceURL, postID,
	)
}

func (c *SQLitePaginationCache) Flush(ctx context.Context, sourceURL string) error {
	_, err := c.db.ExecContext(ctx,
		`DELETE FROM post_seen_cache WHERE source_url=?`, sourceURL)
	return err
}

func (c *SQLitePaginationCache) PruneOlderThan(ctx context.Context, age time.Duration) error {
	cutoff := time.Now().UTC().Add(-age).Format(time.RFC3339)
	_, err := c.db.ExecContext(ctx,
		`DELETE FROM post_seen_cache WHERE seen_at < ?`, cutoff)
	return err
}

// MemoryPaginationCache is an in-memory fallback for use when no DB is available.
type MemoryPaginationCache struct {
	seen map[string]struct{}
}

func NewMemoryPaginationCache() *MemoryPaginationCache {
	return &MemoryPaginationCache{seen: make(map[string]struct{})}
}

func (m *MemoryPaginationCache) Seen(sourceURL, postID string) bool {
	_, ok := m.seen[sourceURL+"::"+postID]
	return ok
}

func (m *MemoryPaginationCache) Mark(sourceURL, postID string) {
	m.seen[sourceURL+"::"+postID] = struct{}{}
}

func (m *MemoryPaginationCache) Flush(_ context.Context, sourceURL string) error {
	for k := range m.seen {
		if len(k) > len(sourceURL) && k[:len(sourceURL)] == sourceURL {
			delete(m.seen, k)
		}
	}
	return nil
}

func (m *MemoryPaginationCache) PruneOlderThan(_ context.Context, _ time.Duration) error {
	return nil
}

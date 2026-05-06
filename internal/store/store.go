package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store provides database access for the scraper system.
type Store struct {
	db     *sql.DB
	encKey string // AES-256-GCM key for sensitive fields; empty = no encryption
}

// New creates a new Store, initializing the SQLite database and running migrations.
func New(dbPath string) (*Store, error) {
	// Ensure data directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// busy_timeout=15000ms gives concurrent writers ~15s to wait for a
	// held write lock before SQLITE_BUSY surfaces. CI machines under
	// load still flaked at 5s when 8+ goroutines raced
	// QueueOutboundForOrg; combined with retryOnBusy in helpers.go this
	// is the belt-and-braces fix.
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(15000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for packages that need direct SQL access
// (e.g. session.StateMachine, session.CheckpointManager).
func (s *Store) DB() *sql.DB { return s.db }

// SetEncryptionKey sets the AES-256-GCM key used to encrypt sensitive DB fields
// (cookies_json, proxy_url). Must be called before any account operations.
func (s *Store) SetEncryptionKey(key string) {
	s.encKey = key
}

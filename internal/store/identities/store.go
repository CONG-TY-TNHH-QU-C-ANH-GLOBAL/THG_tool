// Package identities owns Facebook account identity records:
// the accounts table (with encrypted cookies/credentials) and the
// derived Facebook-status summary. Per DOMAINS.md identities sits
// upstream of outbound/connectors but downstream of users — it is the
// tenant-bounded view of "which FB accounts does this org operate."
//
// Phase 6 extraction (2026-05-22): clean-cut mechanical move.
// Scope-limited to the two *Store-receiver files (accounts.go,
// facebook_status.go). The legacy `sessions.go` / `session_status.go`
// / `identities.go` files use *AppStore and belong to the app domain
// (Phase 11).
//
// encKey threading: cookies_json + proxy_url are encrypted at rest with
// the AES-256-GCM key configured at boot. Identities.Store holds its
// own copy of the key so it can encrypt without reaching back into the
// parent store. Top-level Store.SetEncryptionKey propagates the key
// to this subpackage at the same call site.
package identities

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store provides identities-domain data access. Wired into the
// top-level store.Store via composition; reached via
// `store.Store.Identities()`.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
	encKey  string
}

// NewStore constructs an identities Store with the given encryption
// key. The top-level store.New is the only legitimate production
// caller. Pass an empty key when encryption is disabled (dev mode).
func NewStore(db *sql.DB, dialect dbutil.Dialect, encKey string) *Store {
	return &Store{db: db, dialect: dialect, encKey: encKey}
}

// SetEncryptionKey rotates the AES-256-GCM key. Called by the parent
// Store's SetEncryptionKey so the two stay in sync.
func (s *Store) SetEncryptionKey(key string) { s.encKey = key }

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

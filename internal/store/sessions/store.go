// Package sessions owns the browser_sessions table: one row per account's
// Chrome/Docker browser session lifecycle (CDP/VNC ports, status,
// last-active tracking) plus the LocalSessionStatus enum used by the Chrome
// Extension connector and workspace dashboards.
//
// PR1 (2026-07-01): mechanical move from the legacy top-level *AppStore
// receiver (internal/store/sessions.go + session_status.go). Zero semantic
// change — same queries, same table, same column order, same scan logic.
// The old top-level files now hold source-compat type aliases + *AppStore
// bridge methods; callers migrate incrementally, no big-bang required.
package sessions

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store provides sessions-domain data access. Wired into the top-level
// store.Store via composition; reached via `store.Store.Sessions()`.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a sessions Store. The top-level store.New is the
// only legitimate production caller.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour. Carrying parity with sibling subpackages.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

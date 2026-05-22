// Package connectors owns the Chrome-extension bridge tables:
// connector_commands, connector_screenshots, agent_tokens (claim path),
// connector_streams ownership, selector cache. Per DOMAINS.md the
// connectors domain sits ABOVE identities + outbound but is a pure
// transport layer — every method is bounded by (org_id, agent_id).
//
// Phase 7 extraction (2026-05-22): mechanical clean-cut move from
// top-level store. Zero semantic change.
package connectors

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store provides connector-domain data access. Wired into the
// top-level store.Store via composition; reached via
// `store.Store.Connectors()`.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a connectors Store. The top-level store.New is
// the only legitimate production caller.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour. Carrying parity with sibling subpackages.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

// Package threads owns conversation_threads + conversation_messages —
// the inbox-style state for outbound DMs and inbound replies.
//
// Phase 8a extraction (2026-05-22): mechanical clean-cut move.
// outbound's CheckConversationGate hook reads thread state via the
// conversationGateForOutbound adapter at the parent store level —
// keeping the adapter at the top-level avoids a dependency loop.
package threads

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store provides threads-domain data access.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a threads Store.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

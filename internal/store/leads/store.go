// Package leads owns the lead pipeline: leads + lead_engagement
// projection + classification_log + context_niches.
//
// Phase 8b extraction (2026-05-22) — last and hardest of the wave.
// Cross-domain SQL (EXISTS sub-projections into outbound_messages +
// engagement projection joining action_ledger / execution_attempts)
// is carried as-is per [[feedback_no_bidirectional_domain_knowledge]]
// §2.2 (cross-domain reads accepted as projections with `// tenant-ok`
// annotation). The mechanical move preserves byte-for-byte behaviour;
// the cross-domain SQL cleanup is a separate refactor.
package leads

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
	"github.com/thg/scraper/internal/store/threads"
)

// Store provides leads-domain data access. Holds a *threads.Store
// reference for the engagement-projection cross-domain reads (per
// DOMAINS.md §2.2 cross-domain projections are accepted; per L1
// leads is downstream of threads so the import direction is legal).
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
	threads *threads.Store
}

// NewStore constructs a leads Store with a threads handle injected for
// engagement-projection cross-domain reads.
func NewStore(db *sql.DB, dialect dbutil.Dialect, threadsStore *threads.Store) *Store {
	return &Store{db: db, dialect: dialect, threads: threadsStore}
}

// Threads returns the injected threads-domain handle for internal use
// by the engagement projection. NOT a public re-export — callers should
// reach threads via store.Threads(), not via leads.Threads().
func (s *Store) Threads() *threads.Store { return s.threads }

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

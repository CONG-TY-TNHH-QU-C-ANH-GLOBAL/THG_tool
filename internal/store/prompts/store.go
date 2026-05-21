// Package prompts owns the AI-prompt-machinery tables: prompt_logs,
// scan_logs, ai_memory, prompt_routing observability rows, org_skills,
// skill_executions. Per DOMAINS.md §2 prompts is upper-tier (consumed
// by AI/orchestrator code paths) — imports dbutil + lower domains only.
//
// Phase 9 extraction (2026-05-21): mechanical clean-cut move from
// top-level store. Zero semantic change. Method bodies preserved
// byte-for-byte; method names unchanged so the migration cost lives
// entirely at the caller-import level.
package prompts

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store provides prompt-domain data access. Composed under the parent
// store.Store; reached via `store.Store.Prompts()`.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a prompts Store. The top-level store.New is the
// only legitimate production caller.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// DB returns the underlying *sql.DB. Reserved for test code paths;
// production callers go through typed methods.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour used by this Store. Prompts has no
// dialect-divergent SQL today; carrying the dialect for parity with
// sibling subpackages.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

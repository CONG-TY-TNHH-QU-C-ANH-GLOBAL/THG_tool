// Package app owns the heterogeneous "application-side" tables that
// do not fit a tighter domain: career_jobs, kpi, learning,
// media_assets, price_items, stats.
//
// Phase 11 narrow-scope extraction (2026-05-22): only the
// *Store-receiver files moved. The legacy *AppStore wrapper
// (`app_store.go` at top-level) is a different concern — it owns the
// app_tasks + task_leads tables consumed by the connector pipeline
// and stays where it is until its own refactor decides whether to
// fold into here.
package app

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Store provides app-domain data access.
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs an app Store.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

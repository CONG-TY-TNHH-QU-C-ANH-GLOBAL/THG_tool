// Package app owns the heterogeneous "application-side" tables that
// do not fit a tighter domain: career_jobs, kpi, media_assets,
// price_items, stats — plus, since the AppStore dissolution (PR6,
// 2026-07-05), the app_tasks + task_leads cluster consumed by the
// connector pipeline (tasks.go, task_leads.go) and the idempotent
// bootstrap for the app/browser-infra tables (migrate.go, called by
// store.New). The legacy top-level *AppStore wrapper is gone.
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

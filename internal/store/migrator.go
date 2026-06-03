// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// schema_migrations table — the version registry. Each row is one
// migration that has been applied to this database. Created on
// demand by the runner; idempotent.
//
// Why we built this in-house rather than pulling golang-migrate:
//   - Our migration surface is tiny (one baseline + a handful of
//     future incrementals).
//   - We need a non-standard "baseline marker" behaviour for
//     existing SQLite installs that already have a schema but no
//     migrations table — see [recordBaselineIfNeeded].
//   - Zero new module dependencies. POSTGRES_COMPAT_PLAN §3.5
//     defended bringing in golang-migrate; the call here is the
//     opposite trade — less surface area, less maintained code in
//     the dependency graph.
//
// If our migration count crosses ~30 or we need transactional rollback
// semantics, swap to golang-migrate. The interface boundary
// ([Migration]) is small enough to make that swap a single-PR job.

// migrationSchema is the DDL for the version registry. Compatible
// with both SQLite and Postgres without dialect helpers.
const migrationSchema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`

// Migration is one versioned schema change. Up SQL only — we do not
// support down migrations. Production-grade rollback in a multi-team
// system means restoring from backup, not running a reverse script
// that depends on database state being intact.
//
// Version numbers are monotonic integers starting at 1. The runner
// applies in ascending order, skipping any version already recorded
// in schema_migrations.
type Migration struct {
	Version int
	Name    string
	// SQL is the raw DDL/DML for this migration. It may contain multiple
	// `;`-separated statements (modernc/sqlite + pgx both execute a
	// multi-statement body in one Exec). By DEFAULT the runner wraps the
	// whole body + the schema_migrations version record in ONE transaction
	// (atomic, fail-fast, no half-applied state). A migration that cannot
	// run inside a transaction — e.g. Postgres `CREATE INDEX CONCURRENTLY`
	// — opts out by putting `-- migrate:notx` on its first comment line.
	SQL string
}

// Embed the entire directory (not `migrations/*.sql`) so the embed
// pattern is satisfied even before any migration files exist. The
// loader filters by `.up.sql` suffix at runtime, ignoring README.md
// and anything else that lands here.
//
//go:embed migrations
var migrationFS embed.FS

// loadMigrations parses the embedded migrations/ directory.
// Files MUST be named `NNNN_description.up.sql` where NNNN is a
// zero-padded version number (e.g. `0002_add_vector_column.up.sql`).
// The dialect-specific suffix `__sqlite.up.sql` or `__postgres.up.sql`
// lets the runner pick the right variant; files without a suffix are
// considered dialect-portable.
//
// This is the entire migration-file format. No metadata header, no
// `BEGIN/COMMIT` ceremony — the script writes the SQL it needs to run.
func loadMigrations(dialectName string) ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		// No migrations directory is acceptable in dev — there are no
		// non-baseline migrations yet.
		return nil, nil
	}
	out := make([]Migration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		// Filter by dialect: if the filename includes __<other-dialect>,
		// skip; if it includes __<this-dialect>, take; if no suffix,
		// take as portable.
		base := strings.TrimSuffix(e.Name(), ".up.sql")
		if strings.Contains(base, "__sqlite") && dialectName != "sqlite" {
			continue
		}
		if strings.Contains(base, "__postgres") && dialectName != "postgres" {
			continue
		}
		// Strip the dialect suffix to get the canonical name.
		canonical := strings.NewReplacer("__sqlite", "", "__postgres", "").Replace(base)

		parts := strings.SplitN(canonical, "_", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("migration filename %q must be NNNN_name.up.sql", e.Name())
		}
		var v int
		if _, err := fmt.Sscanf(parts[0], "%d", &v); err != nil {
			return nil, fmt.Errorf("migration filename %q has non-numeric version", e.Name())
		}
		body, err := fs.ReadFile(migrationFS, "migrations/"+e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Version: v, Name: parts[1], SQL: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// runMigrations is the top-level entry point called by store.New
// AFTER the legacy [Store.migrate] has bootstrapped (or, on a brand-
// new PG database, has done nothing). It:
//
//  1. Ensures schema_migrations exists.
//  2. Records the baseline (version 1) if we detect existing tables
//     without a corresponding migration record.
//  3. Loads embedded migrations and applies any not yet recorded.
//
// Failures here abort store boot. Boot-time migration failures are
// production incidents — we deliberately do NOT swallow them.
func (s *Store) runMigrations(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migrationSchema); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	if err := s.recordBaselineIfNeeded(ctx); err != nil {
		return fmt.Errorf("baseline: %w", err)
	}

	migrations, err := loadMigrations(s.dialect.Name())
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	applied, err := s.appliedMigrationVersions(ctx)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if _, done := applied[m.Version]; done {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			return fmt.Errorf("apply migration %04d_%s: %w", m.Version, m.Name, err)
		}
	}
	return nil
}

// recordBaselineIfNeeded marks the existing schema as version 1 when
// we detect a pre-migrator database. Heuristic: if any known
// long-lived table (`knowledge_sources`) exists but the migrations
// table is empty, the database was bootstrapped via the legacy
// [Store.migrate] path and is therefore at "version 1" by definition.
//
// On a brand-new database (no tables, fresh PG instance), this
// function is a no-op — the migration runner will then APPLY the
// 0001 migration file from disk to bring the schema up.
//
// Risk addressed: R11 in POSTGRES_COMPAT_PLAN.md (existing prod
// SQLite installs MUST NOT have the baseline migration re-run, or
// CREATE TABLE statements would fail on duplicate-table errors).
func (s *Store) recordBaselineIfNeeded(ctx context.Context) error {
	// Are there any rows in schema_migrations already?
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	// Does the legacy schema exist? Probe via a known table. If the
	// table doesn't exist we're on a fresh DB — no baseline to record;
	// the 0001 migration (if present) will create the schema.
	if !s.tableExists(ctx, "knowledge_sources") {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES (1, 'baseline_legacy_schema')`,
	)
	return err
}

// tableExists tries a portable existence probe. SQLite and Postgres
// both implement the standard `information_schema.tables` view; we
// use it instead of dialect-specific catalog queries.
func (s *Store) tableExists(ctx context.Context, name string) bool {
	q := s.dialect.Rebind(`
		SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_name = ?`)
	var n int
	if err := s.db.QueryRowContext(ctx, q, name).Scan(&n); err == nil {
		return n > 0
	}
	// Fallback for SQLite older than the version that supports
	// information_schema (rarely encountered with modernc/sqlite, but
	// belt-and-braces). sqlite_master is the SQLite-native catalog.
	if s.dialect.Name() == "sqlite" {
		row := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name)
		if err := row.Scan(&n); err == nil {
			return n > 0
		}
	}
	return false
}

func (s *Store) appliedMigrationVersions(ctx context.Context) (map[int]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]struct{}{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = struct{}{}
	}
	return out, rows.Err()
}

// execer is satisfied by both *sql.DB and *sql.Tx so recordMigrationVersion
// can run inside or outside a transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// applyMigration runs one migration ATOMICALLY: the migration body and its
// schema_migrations version record commit together inside a single transaction,
// or both roll back. This removes the old worst-case (body succeeded but the
// version record failed → next boot re-runs a non-idempotent migration) and
// makes a crash mid-migration leave the DB clean (no partial half-state, no
// version row). Fail-fast — any error aborts boot (boot-time migration failures
// are production incidents, never swallowed).
//
// Opt-out: a migration whose first line is `-- migrate:notx` runs WITHOUT a
// transaction. This is the escape hatch for operations that cannot run inside a
// transaction (e.g. Postgres `CREATE INDEX CONCURRENTLY`). modernc/sqlite
// supports multi-statement Exec and DDL-in-transaction (verified), so SQLite
// migrations default to transactional.
func (s *Store) applyMigration(ctx context.Context, m Migration) error {
	if migrationOptsOutOfTx(m.SQL) {
		if _, err := s.db.ExecContext(ctx, m.SQL); err != nil {
			return err
		}
		return s.recordMigrationVersion(ctx, s.db, m)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return err // fail-fast; defer rolls back the partial body
	}
	if err := s.recordMigrationVersion(ctx, tx, m); err != nil {
		return err
	}
	return tx.Commit()
}

// migrationOptsOutOfTx reports whether a migration declares `-- migrate:notx`
// on (any of) its first non-empty lines — the explicit escape hatch for DDL
// that cannot run inside a transaction.
func migrationOptsOutOfTx(sqlText string) bool {
	for _, line := range strings.Split(sqlText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "--") {
			if strings.Contains(strings.ToLower(line), "migrate:notx") {
				return true
			}
			continue
		}
		return false // first real SQL line — no directive
	}
	return false
}

func (s *Store) recordMigrationVersion(ctx context.Context, ex execer, m Migration) error {
	if _, err := ex.ExecContext(ctx,
		s.dialect.Rebind(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`),
		m.Version, m.Name, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("record migration version: %w", err)
	}
	return nil
}

// CurrentSchemaVersion reports the highest version recorded in
// schema_migrations. Useful for /healthz / metrics so dashboards can
// alert when production drifts behind expected version.
func (s *Store) CurrentSchemaVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

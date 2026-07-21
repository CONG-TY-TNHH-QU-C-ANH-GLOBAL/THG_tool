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
// Files may live directly in migrations/ or in domain/plane
// subdirectories (e.g. migrations/platform/) — the directory is purely
// organizational; ordering is ALWAYS global by NNNN, and a (version,
// dialect) collision anywhere in the tree is a load error, never a
// nondeterministic apply order.
//
// This is the entire migration-file format. No metadata header, no
// `BEGIN/COMMIT` ceremony — the script writes the SQL it needs to run.
func loadMigrations(dialectName string) ([]Migration, error) {
	return loadMigrationsFrom(migrationFS, "migrations", dialectName)
}

// loadMigrationsFrom is loadMigrations over an explicit fs.FS root, so
// the discovery/ordering/collision rules are unit-testable with fstest.
func loadMigrationsFrom(fsys fs.FS, root, dialectName string) ([]Migration, error) {
	var out []Migration
	walkErr := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".up.sql") {
			return err
		}
		m, take, parseErr := parseMigrationFilename(d.Name(), dialectName)
		if parseErr != nil || !take {
			return parseErr
		}
		body, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return readErr
		}
		m.SQL = string(body)
		out = append(out, m)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	// Reject (version, dialect) collisions: with an unstable sort the apply
	// order would be nondeterministic and the second version-record INSERT
	// would fail mid-boot on the schema_migrations primary key. Fail the
	// load with a clear message instead.
	for i := 1; i < len(out); i++ {
		if out[i].Version == out[i-1].Version {
			return nil, fmt.Errorf("duplicate migration version %04d for dialect %s: %q and %q",
				out[i].Version, dialectName, out[i-1].Name, out[i].Name)
		}
	}
	return out, nil
}

// parseMigrationFilename applies the naming + dialect-filter rules to one
// file name. take=false means the file belongs to another dialect.
func parseMigrationFilename(name, dialectName string) (m Migration, take bool, err error) {
	base := strings.TrimSuffix(name, ".up.sql")
	if strings.Contains(base, "__sqlite") && dialectName != "sqlite" {
		return Migration{}, false, nil
	}
	if strings.Contains(base, "__postgres") && dialectName != "postgres" {
		return Migration{}, false, nil
	}
	// Strip the dialect suffix to get the canonical name.
	canonical := strings.NewReplacer("__sqlite", "", "__postgres", "").Replace(base)

	parts := strings.SplitN(canonical, "_", 2)
	if len(parts) < 2 {
		return Migration{}, false, fmt.Errorf("migration filename %q must be NNNN_name.up.sql", name)
	}
	var v int
	if _, err := fmt.Sscanf(parts[0], "%d", &v); err != nil {
		return Migration{}, false, fmt.Errorf("migration filename %q has non-numeric version", name)
	}
	return Migration{Version: v, Name: parts[1]}, true, nil
}

// runMigrationsOn is runMigrations' actual body (see migrator_postgres_lock.go
// for the Postgres advisory-lock dispatcher), parameterized over the
// executor so the Postgres path can pin it to one locked connection while
// SQLite keeps using the pool exactly as before.
func (s *Store) runMigrationsOn(ctx context.Context, ex dbExecer) error {
	if _, err := ex.ExecContext(ctx, migrationSchema); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	if err := s.recordBaselineIfNeeded(ctx, ex); err != nil {
		return fmt.Errorf("baseline: %w", err)
	}

	migrations, err := loadMigrations(s.dialect.Name())
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	applied, err := s.appliedMigrationVersions(ctx, ex)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if _, done := applied[m.Version]; done {
			continue
		}
		if err := s.applyMigration(ctx, ex, m); err != nil {
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
// Risk addressed: R11 in specs/domains/knowledge-platform/features/knowledge-os/implementation/postgres-compat.md (existing prod
// SQLite installs MUST NOT have the baseline migration re-run, or
// CREATE TABLE statements would fail on duplicate-table errors).
func (s *Store) recordBaselineIfNeeded(ctx context.Context, ex dbExecer) error {
	// Are there any rows in schema_migrations already?
	var count int
	if err := ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	// Does the legacy schema exist? Probe via a known table. If the
	// table doesn't exist we're on a fresh DB — no baseline to record;
	// the 0001 migration (if present) will create the schema.
	if !s.tableExistsOn(ctx, ex, "knowledge_sources") {
		return nil
	}
	_, err := ex.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES (1, 'baseline_legacy_schema')`,
	)
	return err
}

// tableExists tries a portable existence probe against the store's pool.
// Safe for post-boot introspection (tests, health checks) — NOT used inside
// the locked Postgres migration path, which must stay on one connection;
// see tableExistsOn.
func (s *Store) tableExists(ctx context.Context, name string) bool {
	return s.tableExistsOn(ctx, s.db, name)
}

// tableExistsOn is tableExists parameterized over the executor, so the
// locked Postgres migration path (runMigrationsOn) can probe on the SAME
// connection that holds the advisory lock instead of the pool.
func (s *Store) tableExistsOn(ctx context.Context, ex dbExecer, name string) bool {
	q := s.dialect.Rebind(`
		SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_name = ?`)
	var n int
	if err := ex.QueryRowContext(ctx, q, name).Scan(&n); err == nil {
		return n > 0
	}
	// Fallback for SQLite older than the version that supports
	// information_schema (rarely encountered with modernc/sqlite, but
	// belt-and-braces). sqlite_master is the SQLite-native catalog.
	if s.dialect.Name() == "sqlite" {
		row := ex.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name)
		if err := row.Scan(&n); err == nil {
			return n > 0
		}
	}
	return false
}

func (s *Store) appliedMigrationVersions(ctx context.Context, ex dbExecer) (map[int]struct{}, error) {
	rows, err := ex.QueryContext(ctx, `SELECT version FROM schema_migrations`)
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
func (s *Store) applyMigration(ctx context.Context, ex dbExecer, m Migration) error {
	if migrationOptsOutOfTx(m.SQL) {
		if _, err := ex.ExecContext(ctx, m.SQL); err != nil {
			return err
		}
		return s.recordMigrationVersion(ctx, ex, m)
	}
	tx, err := ex.BeginTx(ctx, nil)
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
	for line := range strings.SplitSeq(sqlText, "\n") {
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

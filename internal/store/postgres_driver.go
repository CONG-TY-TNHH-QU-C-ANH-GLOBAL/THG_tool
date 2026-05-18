package store

// postgresDriverName is the database/sql driver name the store uses
// when DATABASE_URL points to Postgres. The driver itself is NOT
// imported here — keeping the dependency optional means the SQLite-
// only builds (dev, tests, hot-fix branches) do not have to pull pgx
// into their dependency graph.
//
// To enable PG support in a binary, add a blank import to your main
// package:
//
//	// In cmd/scraper/main.go (or wherever your binary's main lives):
//	import _ "github.com/jackc/pgx/v5/stdlib"
//
// The pgx stdlib package registers itself under the "pgx" driver name
// on init. If you forget the import, store.New() with a PG DSN fails
// at sql.Open with "unknown driver pgx" — clear enough.
//
// Alternative driver names (use only if you switch away from pgx):
//   - "postgres" (lib/pq, legacy; older project)
//   - "pgx"      (pgx/v5/stdlib, recommended)
//
// We pick pgx because it has better performance, supports the COPY
// protocol for batch ingest (relevant when the Knowledge OS sync
// pipeline scales), and is maintained actively.
const postgresDriverName = "pgx"

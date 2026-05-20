package dbutil

import (
	"strings"
	"testing"
)

// Rebind on Postgres rewrites every `?` to `$N` in left-to-right order.
// Load-bearing correctness property for the PG migration risk inventory.
func TestPostgresDialect_Rebind(t *testing.T) {
	pg := PostgresDialect{}
	cases := []struct {
		in   string
		want string
	}{
		{"SELECT 1", "SELECT 1"},
		{"SELECT * FROM t WHERE a = ?", "SELECT * FROM t WHERE a = $1"},
		{"INSERT INTO t (a, b, c) VALUES (?, ?, ?)", "INSERT INTO t (a, b, c) VALUES ($1, $2, $3)"},
		{"UPDATE t SET a = ?, b = ? WHERE id = ? AND org_id = ?",
			"UPDATE t SET a = $1, b = $2 WHERE id = $3 AND org_id = $4"},
		{"-- comment with ? in it\nSELECT ?", "-- comment with $1 in it\nSELECT $2"},
	}
	for _, c := range cases {
		got := pg.Rebind(c.in)
		if got != c.want {
			t.Errorf("Rebind(%q):\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}

// Rebind on SQLite is the identity transform.
func TestSQLiteDialect_RebindIsIdentity(t *testing.T) {
	sqlite := SQLiteDialect{}
	inputs := []string{
		"SELECT 1",
		"SELECT * FROM t WHERE a = ?",
		"INSERT INTO t VALUES (?, ?, ?)",
	}
	for _, in := range inputs {
		if got := sqlite.Rebind(in); got != in {
			t.Errorf("SQLite Rebind should be identity; got %q", got)
		}
	}
}

// IntervalDaysExpr produces a dialect-native expression. Tested as
// substring match — we don't pin exact formatting since both forms
// have whitespace variations that produce identical SQL behavior.
func TestIntervalDaysExpr(t *testing.T) {
	if got := (SQLiteDialect{}).IntervalDaysExpr(30); !strings.Contains(got, "DATETIME") || !strings.Contains(got, "30") {
		t.Errorf("SQLite interval should reference DATETIME and 30; got %q", got)
	}
	if got := (PostgresDialect{}).IntervalDaysExpr(30); !strings.Contains(got, "INTERVAL") || !strings.Contains(got, "30") {
		t.Errorf("PG interval should reference INTERVAL and 30; got %q", got)
	}
}

// NowExpr returns the dialect's idiomatic "current timestamp"
// expression. Both are SQL-standard but stored differently in the
// helper so future dialect-specific tuning has a single touchpoint.
func TestNowExpr(t *testing.T) {
	if got := (SQLiteDialect{}).NowExpr(); got != "CURRENT_TIMESTAMP" {
		t.Errorf("SQLite NowExpr: got %q", got)
	}
	if got := (PostgresDialect{}).NowExpr(); got != "NOW()" {
		t.Errorf("PG NowExpr: got %q", got)
	}
}

// Names are stable contract values — log + metric backends key on them.
func TestDialectNames(t *testing.T) {
	if (SQLiteDialect{}).Name() != "sqlite" {
		t.Error("SQLite dialect Name() must be 'sqlite'")
	}
	if (PostgresDialect{}).Name() != "postgres" {
		t.Error("Postgres dialect Name() must be 'postgres'")
	}
}

// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// Characterization tests for the migration loader topology (database
// boundary sprint PR2): discovery may be organized in domain/plane
// subdirectories, but ordering is ALWAYS global by NNNN version and a
// (version, dialect) collision is a load error — never a
// nondeterministic apply order.

func mapFS(files map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for name, body := range files {
		out[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

func versionsOf(ms []Migration) []int {
	out := make([]int, len(ms))
	for i, m := range ms {
		out[i] = m.Version
	}
	return out
}

// TestLoadMigrations_SubdirectoriesGlobalOrder pins the modular layout:
// files in subdirectories are discovered, and ordering interleaves flat +
// subdirectory files strictly by version.
func TestLoadMigrations_SubdirectoriesGlobalOrder(t *testing.T) {
	fsys := mapFS(map[string]string{
		"migrations/0001_base.up.sql":              "SELECT 1;",
		"migrations/platform/0003_platform.up.sql": "SELECT 3;",
		"migrations/local/0002_local.up.sql":       "SELECT 2;",
		"migrations/README.md":                     "not a migration",
	})
	ms, err := loadMigrationsFrom(fsys, "migrations", "sqlite")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := versionsOf(ms)
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("global version order broken across subdirectories: %v", got)
	}
	if ms[1].Name != "local" || ms[2].Name != "platform" {
		t.Fatalf("names mis-parsed: %+v", ms)
	}
}

// TestLoadMigrations_DuplicateVersionRejected pins the collision guard:
// two same-dialect files with one version must fail the load with a
// clear error, wherever they live in the tree.
func TestLoadMigrations_DuplicateVersionRejected(t *testing.T) {
	fsys := mapFS(map[string]string{
		"migrations/0002_first.up.sql":           "SELECT 1;",
		"migrations/platform/0002_second.up.sql": "SELECT 2;",
	})
	_, err := loadMigrationsFrom(fsys, "migrations", "sqlite")
	if err == nil || !strings.Contains(err.Error(), "duplicate migration version 0002") {
		t.Fatalf("duplicate version must be rejected with a clear error, got: %v", err)
	}
}

// TestLoadMigrations_DialectFilter pins the suffix rules: a dialect sees
// its own variant plus portable files, never the other dialect's. The
// same version across DIFFERENT dialects is legal (0001 does this today).
func TestLoadMigrations_DialectFilter(t *testing.T) {
	fsys := mapFS(map[string]string{
		"migrations/0001_base__sqlite.up.sql":    "SELECT 1;",
		"migrations/0001_other__postgres.up.sql": "SELECT 1;",
		"migrations/0002_portable.up.sql":        "SELECT 2;",
	})
	for dialect, wantName := range map[string]string{"sqlite": "base", "postgres": "other"} {
		ms, err := loadMigrationsFrom(fsys, "migrations", dialect)
		if err != nil {
			t.Fatalf("%s load: %v", dialect, err)
		}
		if len(ms) != 2 || ms[0].Name != wantName || ms[1].Name != "portable" {
			t.Fatalf("%s dialect filter broken: %+v", dialect, ms)
		}
	}
}

// TestLoadMigrations_MalformedNameRejected moves the malformed-filename
// failure from boot time to test time.
func TestLoadMigrations_MalformedNameRejected(t *testing.T) {
	for _, bad := range []string{"noversion.up.sql", "abcd_name.up.sql"} {
		fsys := mapFS(map[string]string{"migrations/" + bad: "SELECT 1;"})
		if _, err := loadMigrationsFrom(fsys, "migrations", "sqlite"); err == nil {
			t.Errorf("malformed filename %q must be rejected", bad)
		}
	}
}

// TestEmbeddedMigrations_LoadCleanBothDialects validates the REAL
// embedded directory at test time for both dialects: parse clean,
// strictly ascending unique versions, baseline present. A malformed or
// colliding migration file now fails CI instead of production boot.
func TestEmbeddedMigrations_LoadCleanBothDialects(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		ms, err := loadMigrations(dialect)
		if err != nil {
			t.Fatalf("%s: embedded migrations must load clean: %v", dialect, err)
		}
		if len(ms) == 0 || ms[0].Version != 1 {
			t.Fatalf("%s: baseline version 1 missing (got %v)", dialect, versionsOf(ms))
		}
		for i := 1; i < len(ms); i++ {
			if ms[i].Version <= ms[i-1].Version {
				t.Fatalf("%s: versions not strictly ascending: %v", dialect, versionsOf(ms))
			}
		}
	}
}

// TestEmbeddedMigrations_PlatformBaselineDiscovered pins the modular
// PostgreSQL platform baseline (database boundary sprint PR4): the
// migrations/platform/ pieces are discovered through the subdirectory
// loader, postgres-only, and contiguous from version 100 — a missing or
// mis-suffixed platform file fails here, not at a production PG boot.
func TestEmbeddedMigrations_PlatformBaselineDiscovered(t *testing.T) {
	const first, last = 100, 108
	pg, err := loadMigrations("postgres")
	if err != nil {
		t.Fatalf("postgres load: %v", err)
	}
	got := map[int]bool{}
	for _, m := range pg {
		got[m.Version] = true
	}
	for v := first; v <= last; v++ {
		if !got[v] {
			t.Errorf("platform baseline version %04d missing from postgres load", v)
		}
	}
	sqlite, err := loadMigrations("sqlite")
	if err != nil {
		t.Fatalf("sqlite load: %v", err)
	}
	for _, m := range sqlite {
		if m.Version >= first && m.Version <= last {
			t.Errorf("platform baseline version %04d leaked into the sqlite dialect (%s)", m.Version, m.Name)
		}
	}
}

// TestEmbeddedMigrations_NoMegaSchemaFile is the anti-blob guard: every
// migration EXCEPT the two frozen 0001 baselines must stay a small,
// domain-sized piece. If a migration legitimately needs more than the
// budget, split it into numbered domain-owned pieces — do not grow one
// file into a schema dump.
func TestEmbeddedMigrations_NoMegaSchemaFile(t *testing.T) {
	const maxLines = 300
	err := fs.WalkDir(migrationFS, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".up.sql") || strings.HasPrefix(d.Name(), "0001_") {
			return err
		}
		body, readErr := fs.ReadFile(migrationFS, path)
		if readErr != nil {
			return readErr
		}
		if lines := strings.Count(string(body), "\n") + 1; lines > maxLines {
			t.Errorf("migration %s has %d lines (max %d) — split it into domain-owned pieces instead of one schema dump", path, lines, maxLines)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded dir: %v", err)
	}
}

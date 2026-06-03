// Domain: infra
package store

import (
	"path/filepath"
	"testing"
)

// TestMigrate_UpgradeDoesNotReBlacklist pins the schema-hardening fix: one-off
// destructive backfills (auto-blacklisting legacy groups) run ONLY on a fresh
// DB, never on a schemaBootstrapVersion-bump upgrade — otherwise an upgrade
// would clobber an operator's manual change (re-blacklist a group they cleared).
func TestMigrate_UpgradeDoesNotReBlacklist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")

	db, err := New(path) // fresh bootstrap
	if err != nil {
		t.Fatalf("New (fresh): %v", err)
	}
	raw := db.DB()

	// An Auto-detected group exists and an operator has DELIBERATELY un-blacklisted it.
	if _, err := raw.Exec(`INSERT INTO groups (platform, name, url) VALUES ('facebook', 'Auto-detected', 'https://fb.com/groups/keepme')`); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO group_quality (group_id, blacklist, decision)
		SELECT id, 0, 'monitor' FROM groups WHERE name = 'Auto-detected'`); err != nil {
		t.Fatalf("seed group_quality: %v", err)
	}

	// Simulate a version bump: leave the marker table populated but at an OLD
	// version so the next New() re-runs migrate() as an UPGRADE (not fresh).
	if _, err := raw.Exec(`DELETE FROM _schema_bootstrap_marker`); err != nil {
		t.Fatalf("reset marker: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO _schema_bootstrap_marker (version) VALUES (1)`); err != nil {
		t.Fatalf("seed old marker: %v", err)
	}
	_ = db.Close()

	// Re-open → migrate() re-runs (marker version 1 != current). freshDB must be
	// false (marker table has a row), so the blacklist backfill is skipped.
	db2, err := New(path)
	if err != nil {
		t.Fatalf("New (upgrade): %v", err)
	}
	defer db2.Close()

	var blacklist int
	if err := db2.DB().QueryRow(`SELECT gq.blacklist FROM group_quality gq
		JOIN groups g ON g.id = gq.group_id WHERE g.name = 'Auto-detected'`).Scan(&blacklist); err != nil {
		t.Fatalf("read blacklist: %v", err)
	}
	if blacklist != 0 {
		t.Fatalf("upgrade re-blacklisted an operator-cleared group (blacklist=%d); freshDB guard failed", blacklist)
	}
}

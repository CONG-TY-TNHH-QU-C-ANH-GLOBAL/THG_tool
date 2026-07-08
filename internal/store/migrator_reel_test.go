// Domain: infra (see internal/store/DOMAINS.md)
package store

import "testing"

// TestEmbeddedMigrations_ReelFoundationDiscovered pins the Reel Studio PR-R1
// migration (0111_platform_reels__postgres.up.sql): discovered under the
// postgres dialect, and absent from sqlite — reel has no SQLite schema per
// docs/architecture/decisions/ADR-reel-studio-platform-module.md.
func TestEmbeddedMigrations_ReelFoundationDiscovered(t *testing.T) {
	const version = 111

	pg, err := loadMigrations("postgres")
	if err != nil {
		t.Fatalf("postgres load: %v", err)
	}
	found := false
	for _, m := range pg {
		if m.Version == version {
			found = true
		}
	}
	if !found {
		t.Errorf("reel foundation migration %04d missing from postgres load", version)
	}

	sqlite, err := loadMigrations("sqlite")
	if err != nil {
		t.Fatalf("sqlite load: %v", err)
	}
	for _, m := range sqlite {
		if m.Version == version {
			t.Errorf("reel foundation migration %04d leaked into the sqlite dialect (%s)", version, m.Name)
		}
	}
}

// TestEmbeddedMigrations_ReelEnrichedDiscovered pins the Reel Studio PR-E1
// migration (0112_platform_reel_enriched__postgres.up.sql): discovered under
// the postgres dialect, and absent from sqlite — reel has no SQLite schema
// per docs/architecture/decisions/ADR-reel-studio-platform-module.md.
func TestEmbeddedMigrations_ReelEnrichedDiscovered(t *testing.T) {
	const version = 112

	pg, err := loadMigrations("postgres")
	if err != nil {
		t.Fatalf("postgres load: %v", err)
	}
	found := false
	for _, m := range pg {
		if m.Version == version {
			found = true
		}
	}
	if !found {
		t.Errorf("reel enriched migration %04d missing from postgres load", version)
	}

	sqlite, err := loadMigrations("sqlite")
	if err != nil {
		t.Fatalf("sqlite load: %v", err)
	}
	for _, m := range sqlite {
		if m.Version == version {
			t.Errorf("reel enriched migration %04d leaked into the sqlite dialect (%s)", version, m.Name)
		}
	}
}

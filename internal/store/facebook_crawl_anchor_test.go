// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"strings"
	"testing"
)

// TestFacebookCrawlAnchorMigration_Concurrent pins the PR-M2B migration-safety
// contract for the accounts tenant anchor: it is built with a non-transactional
// CONCURRENTLY index so a non-empty production accounts table is not
// write-blocked, it stays fail-visible (no IF NOT EXISTS), and the campaign
// migration no longer creates it.
func TestFacebookCrawlAnchorMigration_Concurrent(t *testing.T) {
	byVersion := map[int]Migration{}
	ms, err := loadMigrations("postgres")
	if err != nil {
		t.Fatalf("load postgres migrations: %v", err)
	}
	for _, m := range ms {
		byVersion[m.Version] = m
	}

	anchor, ok := byVersion[112]
	if !ok {
		t.Fatal("accounts tenant anchor migration 0112 not discovered")
	}
	if !migrationOptsOutOfTx(anchor.SQL) {
		t.Error("anchor migration 0112 must be non-transactional (-- migrate:notx)")
	}
	// The exact phrase also proves fail-visibility: no IF NOT EXISTS token can
	// sit between CONCURRENTLY and the index name.
	if !strings.Contains(anchor.SQL, "CREATE UNIQUE INDEX CONCURRENTLY uq_accounts_org_id_id") {
		t.Error("anchor migration 0112 must build uq_accounts_org_id_id CONCURRENTLY (fail-visible, no IF NOT EXISTS)")
	}

	campaigns, ok := byVersion[113]
	if !ok {
		t.Fatal("campaigns migration 0113 not discovered")
	}
	if strings.Contains(campaigns.SQL, "uq_accounts_org_id_id") {
		t.Error("campaigns migration 0113 must not create the accounts anchor")
	}
}

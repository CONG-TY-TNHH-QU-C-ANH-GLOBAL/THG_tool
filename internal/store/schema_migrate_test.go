// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSchemaBootstrapCreatesAllExpectedTables proves a fresh store.New() builds
// every table the runtime + later migration files depend on — now entirely via
// the 0001_legacy_baseline migration applied by runMigrations() (the in-code
// migrate() bootstrap was retired). The concrete signal: knowledge_assets must
// exist, because migration 0002 (add_embedding_metadata) ALTERs it. Production
// CD failed on 2026-05-19 when a partial bootstrap left knowledge_assets
// uncreated and 0002 then hit "no such table: knowledge_assets"; the atomic
// fail-fast baseline makes that impossible.
func TestSchemaBootstrapCreatesAllExpectedTables(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	for _, table := range []string{
		"groups",             // first table
		"accounts",           // core
		"outbound_messages",  // execution
		"action_ledger",      // coordination/attribution
		"user_execution_context", // PR4
		"knowledge_assets",   // expected by migration 0002
		"knowledge_sources",  // baseline probe
		"knowledge_feedback", // last knowledge table
		"org_skills",         // prompts sub-migration table
		"runtime_events",     // coordination sub-migration table
		"selector_cache",     // connectors sub-migration table
	} {
		if !db.tableExists(context.Background(), table) {
			t.Errorf("table %q missing after fresh bootstrap", table)
		}
	}
}

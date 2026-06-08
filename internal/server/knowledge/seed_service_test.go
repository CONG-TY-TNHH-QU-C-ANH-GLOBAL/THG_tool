package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion/csv"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

func TestBuildSeedCSV(t *testing.T) {
	// Raw CSV passes through.
	if out, err := buildSeedCSV(seedServiceBody{CSV: "title,description\nA,B"}); err != nil || !strings.Contains(out, "A,B") {
		t.Fatalf("raw csv passthrough failed: out=%q err=%v", out, err)
	}
	// Rows render to escaped CSV.
	out, err := buildSeedCSV(seedServiceBody{Rows: []seedRow{{Title: "US Fulfillment", Description: "3-5 ngày, phí từ $4.5"}}})
	if err != nil {
		t.Fatalf("rows->csv: %v", err)
	}
	if !strings.Contains(out, "title,description") || !strings.Contains(out, "US Fulfillment") {
		t.Fatalf("rows->csv missing header/row: %q", out)
	}
	// Empty input → typed error.
	if _, err := buildSeedCSV(seedServiceBody{}); err == nil {
		t.Fatalf("empty input must error")
	}
	// Rows with no title → typed error.
	if _, err := buildSeedCSV(seedServiceBody{Rows: []seedRow{{Description: "x"}}}); err == nil {
		t.Fatalf("titleless rows must error")
	}
}

func bootstrapSeedStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// TestSeedServiceFlow_EndToEnd exercises the full P2b path on a real sqlite:
// upsert a csv source → sync via the csv ingestor → approve → it is retrievable.
// Mirrors the handler core so a config/format regression is caught without HTTP.
func TestSeedServiceFlow_EndToEnd(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapSeedStore, "seed_service")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	reg := ingestion.NewRegistry()
	reg.Register(csv.New())
	disp := &ingestion.Dispatcher{
		Registry: reg,
		Health:   db.Knowledge(),
		WriterFactory: func(src *sources.Source) ingestion.AssetWriter {
			return ingestion.NewStoreAssetWriter(db.Knowledge(), src)
		},
	}

	const orgID = int64(5)
	cfg, _ := json.Marshal(map[string]string{
		"asset_type": "sales_playbook",
		"body":       "title,description\nUS Fulfillment,\"3-5 ngày, phí từ $4.5/order\"\nKho US,\"Kho CA/TX\"",
	})
	src := &sources.Source{OrgID: orgID, Type: sources.SourceCSV, Label: "THG Service Knowledge", ConnectionConfig: cfg, SyncPolicy: sources.SyncManual}
	saved, err := db.Knowledge().UpsertSource(ctx, src)
	if err != nil {
		t.Fatalf("upsert source: %v", err)
	}

	res, err := disp.Run(ctx, saved)
	if err != nil {
		t.Fatalf("dispatcher run: %v", err)
	}
	// csv ingestor counts written rows under AssetsSeen (not AssetsCreated).
	if res.AssetsSeen != 2 {
		t.Fatalf("want 2 assets ingested, got seen=%d (rejected=%d)", res.AssetsSeen, res.AssetsRejected)
	}

	// Newly-ingested assets are pending (not yet retrievable).
	pending, err := db.Knowledge().ListAssetsForOrg(ctx, orgID, assets.ListFilter{
		SourceID: saved.ID, States: []assets.AssetState{assets.StatePending},
	})
	if err != nil || len(pending) != 2 {
		t.Fatalf("want 2 pending assets, got %d err=%v", len(pending), err)
	}

	// Approve them.
	for _, a := range pending {
		if err := db.Knowledge().SetAssetState(ctx, a.ID, orgID, assets.StateApproved); err != nil {
			t.Fatalf("approve asset %d: %v", a.ID, err)
		}
	}
	approved, err := db.Knowledge().ListAssetsForOrg(ctx, orgID, assets.ListFilter{
		SourceID: saved.ID, States: []assets.AssetState{assets.StateApproved},
	})
	if err != nil || len(approved) != 2 {
		t.Fatalf("want 2 approved assets, got %d err=%v", len(approved), err)
	}

	// Re-seed (same label) must reuse the source — no duplicate source row.
	src2 := &sources.Source{OrgID: orgID, Type: sources.SourceCSV, Label: "THG Service Knowledge", ConnectionConfig: cfg, SyncPolicy: sources.SyncManual, ID: saved.ID}
	saved2, err := db.Knowledge().UpsertSource(ctx, src2)
	if err != nil || saved2.ID != saved.ID {
		t.Fatalf("re-seed must reuse source id %d, got %d err=%v", saved.ID, saved2.ID, err)
	}
}

// knowledge_sync is the production-safe operational entry point for the
// read-only THG catalog and Training sources. It uses the same source,
// dispatcher, health, and asset-writer contracts as the HTTP UI; it never
// writes to Hub, Training, CMS, or the public website.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion/rest_json"
	suppliercatalog "github.com/thg/scraper/internal/workspace_knowledge/ingestion/supplier_catalog"
	trainingexport "github.com/thg/scraper/internal/workspace_knowledge/ingestion/training_export"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

const (
	catalogLabel  = "THG Hub POD Catalog (read-only)"
	trainingLabel = "THG Training Sales & Marketing SOP (signed read-only)"
)

func main() {
	_ = godotenv.Load()
	orgDefault, _ := strconv.ParseInt(strings.TrimSpace(os.Getenv("KNOWLEDGE_SYNC_ORG_ID")), 10, 64)
	orgID := flag.Int64("org", orgDefault, "organization ID (or KNOWLEDGE_SYNC_ORG_ID)")
	withTraining := flag.Bool("training", false, "sync signed Training sales/marketing export too")
	approveTraining := flag.Bool("approve-training", false, "approve Training assets after sync")
	flag.Parse()
	if *orgID <= 0 {
		log.Fatal("knowledge_sync: -org or KNOWLEDGE_SYNC_ORG_ID is required")
	}

	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		dbPath = "data/scraper.db"
	}
	db, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("knowledge_sync: open store: %v", err)
	}
	defer db.Close()

	registry := ingestion.NewRegistry()
	registry.Register(rest_json.New())
	registry.Register(trainingexport.New())
	registry.Register(suppliercatalog.New())
	dispatcher := &ingestion.Dispatcher{
		Registry: registry, Health: db.Knowledge(),
		WriterFactory: func(src *sources.Source) ingestion.AssetWriter {
			return ingestion.NewStoreAssetWriter(db.Knowledge(), src)
		},
	}
	ctx := context.Background()

	catalog, err := upsertSource(ctx, db, &sources.Source{
		OrgID: *orgID, Type: sources.SourceRESTJSON, Label: catalogLabel,
		ConnectionConfig: rest_json.ExampleConfigTHGHub(), SyncPolicy: sources.SyncManual,
	})
	if err != nil {
		log.Fatalf("knowledge_sync: configure catalog: %v", err)
	}
	if err := syncAndApprove(ctx, dispatcher, db, catalog, true); err != nil {
		log.Fatalf("knowledge_sync: catalog: %v", err)
	}

	if !*withTraining {
		return
	}
	trainingCfg, _ := json.Marshal(map[string]any{
		"base_url":    "https://training.thgfulfill.com/api/integrations/knowledge-export",
		"secret_file": "/etc/thg-scraper/training_knowledge_export_key",
		"scopes":      []string{"sale", "marketing"}, "timeout_seconds": 20, "max_pages": 20,
	})
	training, err := upsertSource(ctx, db, &sources.Source{
		OrgID: *orgID, Type: sources.SourceTrainingExport, Label: trainingLabel,
		ConnectionConfig: trainingCfg, SyncPolicy: sources.SyncManual,
	})
	if err != nil {
		log.Fatalf("knowledge_sync: configure training: %v", err)
	}
	if err := syncAndApprove(ctx, dispatcher, db, training, *approveTraining); err != nil {
		log.Fatalf("knowledge_sync: training: %v", err)
	}
}

func upsertSource(ctx context.Context, db *store.Store, desired *sources.Source) (*sources.Source, error) {
	if desired == nil {
		return nil, fmt.Errorf("nil source")
	}
	list, err := db.Knowledge().ListSourcesForOrg(ctx, desired.OrgID, sources.ListFilter{})
	if err != nil {
		return nil, err
	}
	for _, existing := range list {
		if existing.Type == desired.Type && existing.Label == desired.Label {
			desired.ID = existing.ID
			break
		}
	}
	return db.Knowledge().UpsertSource(ctx, desired)
}

func syncAndApprove(ctx context.Context, dispatcher *ingestion.Dispatcher, db *store.Store, src *sources.Source, approve bool) error {
	result, err := dispatcher.Run(ctx, src)
	if err != nil {
		return err
	}
	approved := 0
	if approve {
		pending, listErr := db.Knowledge().ListAssetsForOrg(ctx, src.OrgID, assets.ListFilter{SourceID: src.ID, States: []assets.AssetState{assets.StatePending}})
		if listErr != nil {
			return listErr
		}
		for _, asset := range pending {
			if err := db.Knowledge().SetAssetState(ctx, asset.ID, src.OrgID, assets.StateApproved); err != nil {
				return err
			}
			approved++
		}
	}
	fmt.Printf("source=%d label=%q seen=%d rejected=%d approved=%d\n", src.ID, src.Label, result.AssetsSeen, result.AssetsRejected, approved)
	return nil
}

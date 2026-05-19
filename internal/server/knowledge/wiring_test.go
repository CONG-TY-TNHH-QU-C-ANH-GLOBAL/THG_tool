// wiring_test.go asserts that the production wiring PR-3 introduced
// (knowledge_sources HTTP handlers → ingestion.Dispatcher → registered
// adapter → store-backed AssetWriter) actually composes end-to-end.
//
// We DO NOT touch the real *store.Store here — that path is already
// covered by the store package's own tests, and threading a real
// sqlite store through the server-side handlers belongs to a wider
// integration suite. What we DO cover is the dispatcher / registry
// / writer composition that this PR is responsible for.
package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion/rest_json"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// fakeAssetStore satisfies ingestion.AssetStore. Captures every
// UpsertKnowledgeAsset call so tests can assert on what the adapter
// produced after the full Dispatcher → AssetWriter → AssetStore path.
type fakeAssetStore struct {
	mu       sync.Mutex
	upserted []*assets.Asset
}

func (f *fakeAssetStore) UpsertKnowledgeAsset(_ context.Context, a *assets.Asset) (*assets.Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *a
	f.upserted = append(f.upserted, &cp)
	return &cp, nil
}

func (f *fakeAssetStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.upserted)
}

// fakeHealth satisfies ingestion.HealthRecorder. Captures the final
// health status so we can assert the dispatcher persisted the right
// outcome.
type fakeHealth struct {
	mu     sync.Mutex
	last   sources.Health
	called int
}

func (f *fakeHealth) UpdateKnowledgeSourceHealth(_ context.Context, _, _ int64, h sources.Health, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = h
	f.called++
	return nil
}

// thgLikeResponse mirrors the production hub.thgfulfill.com response
// shape so the wiring test exercises the same field_map operators
// will use through the UI in PR-4.
func thgLikeResponse(page, pages int, ids []string) []byte {
	items := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]any{
			"id":        id,
			"sku":       "S" + id,
			"thgSku":    "THG-VN-" + id,
			"name":      "Item " + id,
			"category":  "Apparel",
			"origin":    "VN",
			"priceFrom": 10.0,
			"priceTo":   12.0,
			"currency":  "USD",
			"status":    "Active",
			"images":    []string{"https://cdn.example.com/" + id + ".jpg"},
			"updatedAt": "2026-05-17T15:44:47Z",
		})
	}
	body, _ := json.Marshal(map[string]any{
		"data": items,
		"pagination": map[string]any{
			"page":  page,
			"pages": pages,
			"total": len(items) * pages,
			"limit": 100,
		},
	})
	return body
}

// TestDispatcher_EndToEnd_RegistryToWriter is the core wiring test
// PR-3 adds. It builds the exact dependency graph that router.go
// builds at boot — Registry with rest_json registered, Dispatcher
// with a store-backed-style writer factory — and runs Dispatcher.Run
// against an httptest upstream. The test passes iff:
//
//   1. The registry routed SourceRESTJSON to the rest_json adapter.
//   2. The adapter paginated through both pages of the simulated
//      upstream.
//   3. Every item flowed Adapter → products.Writer → AssetWriter →
//      AssetStore.UpsertKnowledgeAsset.
//   4. The dispatcher persisted a "healthy" status at the end.
//   5. The dispatcher reported AssetsCreated equal to the number of
//      upstream items.
//
// Any future adapter that emits product-catalog assets will pass
// this same test shape — that's why we exercise the full Dispatcher
// rather than calling rest_json.Sync directly.
func TestDispatcher_EndToEnd_RegistryToWriter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write(thgLikeResponse(1, 2, []string{"a1", "a2"}))
		case "2":
			_, _ = w.Write(thgLikeResponse(2, 2, []string{"b1"}))
		default:
			http.Error(w, "bad page", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	// Build the exact wiring router.go uses, but with fake store
	// implementations so the test does not require sqlite.
	registry := ingestion.NewRegistry()
	registry.Register(rest_json.New())

	store := &fakeAssetStore{}
	health := &fakeHealth{}
	dispatcher := &ingestion.Dispatcher{
		Registry: registry,
		Health:   health,
		WriterFactory: func(src *sources.Source) ingestion.AssetWriter {
			return ingestion.NewStoreAssetWriter(store, src)
		},
	}

	// Build a SourceRESTJSON pointing at httptest with the THG-shape
	// preset (re-used here to prove the preset is data, not code —
	// nothing in this test branches on "THG").
	cfgRaw := rest_json.ExampleConfigTHGHub()
	var cfg rest_json.Config
	_ = json.Unmarshal(cfgRaw, &cfg)
	cfg.BaseURL = srv.URL
	cfg.Pagination.MaxPages = 5
	cfgBytes, _ := json.Marshal(cfg)

	src := &sources.Source{
		ID:               42,
		OrgID:            7,
		Type:             sources.SourceRESTJSON,
		Label:            "Wiring test source",
		ConnectionConfig: cfgBytes,
		SyncPolicy:       sources.SyncManual,
		Health:           sources.Health{Status: sources.HealthHealthy},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := dispatcher.Run(ctx, src)
	if err != nil {
		t.Fatalf("dispatcher.Run err: %v", err)
	}

	// AssetsCreated reflects what the adapter reports — 3 items
	// across 2 pages.
	if res.AssetsCreated != 3 {
		t.Fatalf("AssetsCreated = %d, want 3 (errors=%v)", res.AssetsCreated, res.Errors)
	}
	// The full path through the writer landed at the store fake.
	if got := store.count(); got != 3 {
		t.Fatalf("AssetStore.upserted = %d, want 3", got)
	}
	// Dispatcher persisted healthy → first call is "syncing", final
	// is "healthy". health.called == 2.
	if health.called < 2 {
		t.Fatalf("HealthRecorder called %d times, want >=2", health.called)
	}
	if health.last.Status != sources.HealthHealthy {
		t.Fatalf("final health = %q, want %q", health.last.Status, sources.HealthHealthy)
	}

	// Spot-check the org/source binding survived: every persisted
	// asset must carry the writer's org/source IDs and the
	// AssetPODProduct type.
	for i, a := range store.upserted {
		if a.OrgID != src.OrgID {
			t.Fatalf("asset[%d].OrgID = %d, want %d", i, a.OrgID, src.OrgID)
		}
		if a.SourceID != src.ID {
			t.Fatalf("asset[%d].SourceID = %d, want %d", i, a.SourceID, src.ID)
		}
		if a.Type != assets.AssetPODProduct {
			t.Fatalf("asset[%d].Type = %s, want POD_product", i, a.Type)
		}
	}
}

// TestDispatcher_UnknownSourceType reports a permanent error and
// flips the health row to "error" — the contract the operator UI
// will surface to "your source type is not supported on this
// build".
func TestDispatcher_UnknownSourceType(t *testing.T) {
	registry := ingestion.NewRegistry()
	// Intentionally empty — no adapter registered.

	health := &fakeHealth{}
	dispatcher := &ingestion.Dispatcher{
		Registry: registry,
		Health:   health,
		WriterFactory: func(src *sources.Source) ingestion.AssetWriter {
			return ingestion.NewStoreAssetWriter(&fakeAssetStore{}, src)
		},
	}
	src := &sources.Source{
		ID:               1,
		OrgID:            1,
		Type:             sources.SourceRESTJSON,
		Label:            "unregistered",
		ConnectionConfig: json.RawMessage(`{}`),
		SyncPolicy:       sources.SyncManual,
	}
	_, err := dispatcher.Run(context.Background(), src)
	if err == nil {
		t.Fatal("expected error for unregistered source type")
	}
	if health.last.Status != sources.HealthError {
		t.Fatalf("health = %q, want %q", health.last.Status, sources.HealthError)
	}
}

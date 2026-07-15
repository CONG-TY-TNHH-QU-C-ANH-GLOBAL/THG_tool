package export_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/server/export"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

const testKey = "svc-secret"
const testOrg = int64(1)

func newApp(t *testing.T, db *store.Store) *fiber.App {
	t.Helper()
	app := fiber.New()
	export.Routes(app.Group("/api"), export.Deps{DB: db, ServiceKey: testKey, OrgID: testOrg})
	return app
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "export.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func doGet(t *testing.T, app *fiber.App, key string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/export/knowledge-assets", nil)
	if key != "" {
		req.Header.Set("X-Service-Key", key)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func TestExport_RejectsMissingServiceKey(t *testing.T) {
	app := newApp(t, newStore(t))
	if resp := doGet(t, app, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no key: status = %d, want 401", resp.StatusCode)
	}
}

func TestExport_RejectsWrongServiceKey(t *testing.T) {
	app := newApp(t, newStore(t))
	if resp := doGet(t, app, "wrong"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong key: status = %d, want 401", resp.StatusCode)
	}
}

func TestExport_EmptyDBReturnsDone(t *testing.T) {
	app := newApp(t, newStore(t))
	body := decodeOK(t, doGet(t, app, testKey))
	if len(body.Items) != 0 || !body.Done {
		t.Fatalf("empty db: got %d items, done=%v; want 0 items, done=true", len(body.Items), body.Done)
	}
}

func TestExport_ReturnsSeededAssetWithNumericExternalID(t *testing.T) {
	db := newStore(t)
	id := seedOne(t, db, assets.StateHidden)

	body := decodeOK(t, doGet(t, newApp(t, db), testKey))
	if len(body.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(body.Items))
	}
	it := body.Items[0]
	if it.ExternalID == "" || it.ExternalID != itoa(id) {
		t.Errorf("external_id = %q, want stable numeric id %q", it.ExternalID, itoa(id))
	}
	if it.State != string(assets.StateHidden) {
		t.Errorf("state = %q, want %q (export must not filter to approved-only)", it.State, assets.StateHidden)
	}
	if body.NextAfterID != it.ExternalID {
		t.Errorf("next_after_id = %q, want last item external_id %q", body.NextAfterID, it.ExternalID)
	}
}

// --- helpers ---

type wireItem struct {
	ExternalID string `json:"external_id"`
	State      string `json:"state"`
}

type wireResp struct {
	Items       []wireItem `json:"items"`
	NextAfterID string     `json:"next_after_id"`
	Done        bool       `json:"done"`
}

func decodeOK(t *testing.T, resp *http.Response) wireResp {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out wireResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func seedOne(t *testing.T, db *store.Store, state assets.AssetState) int64 {
	t.Helper()
	ctx := context.Background()
	src, err := db.Knowledge().UpsertSource(ctx, &sources.Source{
		OrgID: testOrg, Type: sources.SourceCSV, Label: "t",
		SyncPolicy: sources.SyncManual,
		Health:     sources.Health{Status: sources.HealthHealthy},
	})
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	saved, err := db.Knowledge().UpsertAsset(ctx, &assets.Asset{
		OrgID: testOrg, SourceID: src.ID, ExternalID: "src_ext_1",
		Type: assets.AssetPODProduct, Title: "hidden asset", State: state,
	})
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	return saved.ID
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

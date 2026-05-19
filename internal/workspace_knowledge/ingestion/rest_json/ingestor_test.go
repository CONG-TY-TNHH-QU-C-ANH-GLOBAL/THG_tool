package rest_json

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// fakeWriter captures every asset the adapter persists. Used as the
// AssetWriter the dispatcher would otherwise hand the ingestor — keeps
// tests isolated from the store layer.
type fakeWriter struct {
	mu      sync.Mutex
	written []*assets.Asset
	err     error
}

func (f *fakeWriter) Write(_ context.Context, a *assets.Asset) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	// The dispatcher would set OrgID/SourceID; mimic that here so the
	// assets.Asset.Validate inside any downstream test passes.
	if a.OrgID == 0 {
		a.OrgID = 1
	}
	if a.SourceID == 0 {
		a.SourceID = 1
	}
	f.written = append(f.written, a)
	return nil
}

func (f *fakeWriter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.written)
}

// thgLikePayload mirrors the shape of hub.thgfulfill.com responses so
// the field_map under test exercises the same path operators will use
// in production. Numbers are kept small for fast tests.
func thgLikePayload(page, totalPages int, items []map[string]any) string {
	resp := map[string]any{
		"data": items,
		"pagination": map[string]any{
			"page":  page,
			"limit": 100,
			"total": len(items) * totalPages,
			"pages": totalPages,
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func thgItem(id, sku, name, category, origin string, priceFrom, priceTo float64) map[string]any {
	return map[string]any{
		"id":         id,
		"sku":        sku,
		"thgSku":     "THG-" + origin + "-" + sku,
		"name":       name,
		"category":   category,
		"origin":     origin,
		"priceFrom":  priceFrom,
		"priceTo":    priceTo,
		"currency":   "USD",
		"status":     "Active",
		"sizes":      []string{"M", "L"},
		"colors":     []string{},
		"images":     []string{"https://cdn.example.com/" + id + ".jpg"},
		"updatedAt":  "2026-05-17T15:44:47.932Z",
	}
}

// makeSource produces the sources.Source the ingestor expects.
func makeSource(cfgRaw json.RawMessage) *sources.Source {
	return &sources.Source{
		ID:               1,
		OrgID:            42,
		Type:             sources.SourceRESTJSON,
		Label:            "Test catalog",
		ConnectionConfig: cfgRaw,
		SyncPolicy:       sources.SyncManual,
	}
}

// ── Tests ─────────────────────────────────────────────────────────

func TestSync_PagedHappyPath(t *testing.T) {
	pages := [][]map[string]any{
		{
			thgItem("p1", "RHLG5I", "Stainless Cup", "Home & Living", "US", 22, 22),
			thgItem("p2", "BANDANA", "Satin Bandana", "Apparel", "VN", 17.88, 24.96),
		},
		{
			thgItem("p3", "POLO", "Football Polo", "Apparel", "VN", 20.10, 23.48),
		},
	}
	totalPages := len(pages)

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		pageStr := r.URL.Query().Get("page")
		page, _ := strconv.Atoi(pageStr)
		if page < 1 || page > totalPages {
			http.Error(w, "bad page", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(thgLikePayload(page, totalPages, pages[page-1])))
	}))
	defer srv.Close()

	cfgRaw := newTestConfigForTHGShape(srv.URL)
	ing := New()
	fw := &fakeWriter{}
	res, err := ing.Sync(context.Background(), makeSource(cfgRaw), fw)
	if err != nil {
		t.Fatalf("Sync err: %v", err)
	}
	if got := int(atomic.LoadInt32(&hits)); got != totalPages {
		t.Fatalf("http hits = %d, want %d", got, totalPages)
	}
	wantSeen := 3
	if res.AssetsSeen != wantSeen {
		t.Fatalf("AssetsSeen = %d, want %d", res.AssetsSeen, wantSeen)
	}
	if res.AssetsCreated != wantSeen {
		t.Fatalf("AssetsCreated = %d, want %d", res.AssetsCreated, wantSeen)
	}
	if res.AssetsRejected != 0 {
		t.Fatalf("AssetsRejected = %d, want 0; errors=%+v", res.AssetsRejected, res.Errors)
	}
	if fw.count() != wantSeen {
		t.Fatalf("writer.count = %d, want %d", fw.count(), wantSeen)
	}
	// Spot-check first asset.
	first := fw.written[0]
	if first.Type != assets.AssetPODProduct {
		t.Fatalf("asset type = %s, want POD_product", first.Type)
	}
	if first.ExternalID != "p1" {
		t.Fatalf("first.ExternalID = %s, want p1", first.ExternalID)
	}
	if first.Title != "Stainless Cup" {
		t.Fatalf("first.Title = %s, want Stainless Cup", first.Title)
	}
}

func TestSync_StopsWhenLastPageEmpty(t *testing.T) {
	// Upstream lies about pages.pages = 99 but starts returning empty
	// data on page 2. The adapter should stop on the empty page.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(thgLikePayload(1, 99, []map[string]any{
				thgItem("p1", "S1", "Item 1", "Apparel", "VN", 10, 10),
			})))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(thgLikePayload(page, 99, []map[string]any{})))
	}))
	defer srv.Close()

	res, err := New().Sync(context.Background(), makeSource(newTestConfigForTHGShape(srv.URL)), &fakeWriter{})
	if err != nil {
		t.Fatalf("Sync err: %v", err)
	}
	if res.AssetsCreated != 1 {
		t.Fatalf("AssetsCreated = %d, want 1", res.AssetsCreated)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("hits = %d, want 2 (page1 + empty page2)", hits)
	}
}

func TestSync_MaxPagesCeiling(t *testing.T) {
	// Pretend every page is non-empty and "pages": 9999 — we hit MaxPages.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(thgLikePayload(page, 9999, []map[string]any{
			thgItem(fmt.Sprintf("p%d", page), "S", "Item", "Apparel", "VN", 1, 1),
		})))
	}))
	defer srv.Close()

	cfgRaw := newTestConfigForTHGShape(srv.URL)
	// Force a tiny MaxPages via re-marshalling.
	var cfg Config
	_ = json.Unmarshal(cfgRaw, &cfg)
	cfg.Pagination.MaxPages = 3
	cfgRaw, _ = json.Marshal(cfg)

	res, err := New().Sync(context.Background(), makeSource(cfgRaw), &fakeWriter{})
	if err != nil {
		t.Fatalf("Sync err: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("hits = %d, want 3 (MaxPages ceiling)", got)
	}
	if res.AssetsCreated != 3 {
		t.Fatalf("AssetsCreated = %d, want 3", res.AssetsCreated)
	}
}

func TestSync_AuthBearer_AppliesHeaderFromEnv(t *testing.T) {
	t.Setenv("TEST_TOKEN", "shh-its-secret")
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(thgLikePayload(1, 1, []map[string]any{
			thgItem("p1", "S1", "Item", "Apparel", "VN", 1, 1),
		})))
	}))
	defer srv.Close()

	cfgRaw := newTestConfigForTHGShape(srv.URL)
	var cfg Config
	_ = json.Unmarshal(cfgRaw, &cfg)
	cfg.Auth = AuthConfig{Type: "bearer", TokenEnv: "TEST_TOKEN"}
	cfgRaw, _ = json.Marshal(cfg)

	if _, err := New().Sync(context.Background(), makeSource(cfgRaw), &fakeWriter{}); err != nil {
		t.Fatalf("Sync err: %v", err)
	}
	if want := "Bearer shh-its-secret"; sawAuth != want {
		t.Fatalf("Authorization header = %q, want %q", sawAuth, want)
	}
}

func TestSync_AuthBearer_MissingEnvIsPermanent(t *testing.T) {
	t.Setenv("TEST_TOKEN", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfgRaw := newTestConfigForTHGShape(srv.URL)
	var cfg Config
	_ = json.Unmarshal(cfgRaw, &cfg)
	cfg.Auth = AuthConfig{Type: "bearer", TokenEnv: "TEST_TOKEN"}
	cfgRaw, _ = json.Marshal(cfg)

	_, err := New().Sync(context.Background(), makeSource(cfgRaw), &fakeWriter{})
	if err == nil {
		t.Fatal("expected permanent auth error")
	}
	if ingestion.IsRecoverable(err) {
		t.Fatalf("auth-missing should be permanent, got recoverable: %v", err)
	}
}

func TestSync_5xx_IsRecoverable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream blip", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := New().Sync(context.Background(), makeSource(newTestConfigForTHGShape(srv.URL)), &fakeWriter{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !ingestion.IsRecoverable(err) {
		t.Fatalf("5xx should be recoverable: %v", err)
	}
}

func TestSync_403_IsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := New().Sync(context.Background(), makeSource(newTestConfigForTHGShape(srv.URL)), &fakeWriter{})
	if err == nil {
		t.Fatal("expected error")
	}
	if ingestion.IsRecoverable(err) {
		t.Fatalf("403 should be permanent: %v", err)
	}
}

func TestSync_BadItem_BecomesSyncError_OthersStillIngested(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Two items: one good, one missing id.
		_, _ = w.Write([]byte(thgLikePayload(1, 1, []map[string]any{
			thgItem("good1", "S1", "Good", "Apparel", "VN", 1, 1),
			{"name": "missing id row", "category": "Apparel", "updatedAt": "2026-05-17T00:00:00Z"},
		})))
	}))
	defer srv.Close()

	fw := &fakeWriter{}
	res, err := New().Sync(context.Background(), makeSource(newTestConfigForTHGShape(srv.URL)), fw)
	if err != nil {
		t.Fatalf("Sync err: %v", err)
	}
	if res.AssetsCreated != 1 {
		t.Fatalf("AssetsCreated = %d, want 1", res.AssetsCreated)
	}
	if res.AssetsRejected != 1 {
		t.Fatalf("AssetsRejected = %d, want 1", res.AssetsRejected)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("expected 1 SyncError, got %d", len(res.Errors))
	}
	if fw.count() != 1 {
		t.Fatalf("writer received %d, want 1", fw.count())
	}
}

func TestSync_AvailabilityMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// "Inactive" should map to out_of_stock; an unknown value falls
		// back to the configured default (unknown).
		items := []map[string]any{
			thgItem("p1", "S1", "Active item", "Apparel", "VN", 1, 1),
			func() map[string]any { m := thgItem("p2", "S2", "Inactive item", "Apparel", "VN", 1, 1); m["status"] = "Inactive"; return m }(),
			func() map[string]any { m := thgItem("p3", "S3", "Mystery", "Apparel", "VN", 1, 1); m["status"] = "WhoKnows"; return m }(),
		}
		_, _ = w.Write([]byte(thgLikePayload(1, 1, items)))
	}))
	defer srv.Close()

	fw := &fakeWriter{}
	if _, err := New().Sync(context.Background(), makeSource(newTestConfigForTHGShape(srv.URL)), fw); err != nil {
		t.Fatalf("Sync err: %v", err)
	}
	if fw.count() != 3 {
		t.Fatalf("writer received %d, want 3", fw.count())
	}
	// Inspect the persisted Tags — they include the availability value.
	tagsOf := func(a *assets.Asset) map[string]bool {
		m := map[string]bool{}
		for _, t := range a.Tags {
			m[t] = true
		}
		return m
	}
	if !tagsOf(fw.written[0])["in_stock"] {
		t.Fatalf("first asset should have in_stock tag, got %v", fw.written[0].Tags)
	}
	if !tagsOf(fw.written[1])["out_of_stock"] {
		t.Fatalf("second asset should have out_of_stock tag, got %v", fw.written[1].Tags)
	}
	if !tagsOf(fw.written[2])["unknown"] {
		t.Fatalf("third asset should have unknown tag, got %v", fw.written[2].Tags)
	}
}

func TestSync_NonePagination_SingleFetch(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		// "data" at root.
		body, _ := json.Marshal(map[string]any{
			"data": []map[string]any{
				thgItem("p1", "S1", "Item", "Apparel", "VN", 1, 1),
			},
		})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cfgRaw := newTestConfigForTHGShape(srv.URL)
	var cfg Config
	_ = json.Unmarshal(cfgRaw, &cfg)
	cfg.Pagination = PaginationConfig{Scheme: "none"}
	cfgRaw, _ = json.Marshal(cfg)

	res, err := New().Sync(context.Background(), makeSource(cfgRaw), &fakeWriter{})
	if err != nil {
		t.Fatalf("Sync err: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	if res.AssetsCreated != 1 {
		t.Fatalf("AssetsCreated = %d, want 1", res.AssetsCreated)
	}
}

func TestSync_InvalidConfig_IsPermanent(t *testing.T) {
	_, err := New().Sync(context.Background(), makeSource(json.RawMessage(`{"base_url":"not-a-url"}`)), &fakeWriter{})
	if err == nil {
		t.Fatal("expected error")
	}
	if ingestion.IsRecoverable(err) {
		t.Fatalf("invalid config should be permanent: %v", err)
	}
}

func TestSync_BadJSON_IsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	_, err := New().Sync(context.Background(), makeSource(newTestConfigForTHGShape(srv.URL)), &fakeWriter{})
	if err == nil {
		t.Fatal("expected error")
	}
	if ingestion.IsRecoverable(err) {
		t.Fatalf("malformed JSON should be permanent: %v", err)
	}
}

func TestExampleConfigTHGHub_ParsesAndValidates(t *testing.T) {
	raw := ExampleConfigTHGHub()
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.BaseURL != "https://hub.thgfulfill.com/api/public/catalog" {
		t.Fatalf("unexpected base_url: %s", cfg.BaseURL)
	}
	if cfg.FieldMap.SourceID != "id" {
		t.Fatalf("expected source_id mapping to id")
	}
}

func TestParseConfig_EmptyRejected(t *testing.T) {
	cases := []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("null")}
	for i, c := range cases {
		if _, err := ParseConfig(c); err == nil {
			t.Fatalf("case %d: expected error for empty config", i)
		}
	}
}

func TestWriterWriteError_IsRejectedNotFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(thgLikePayload(1, 1, []map[string]any{
			thgItem("p1", "S1", "Item", "Apparel", "VN", 1, 1),
		})))
	}))
	defer srv.Close()
	fw := &fakeWriter{err: errors.New("simulated")}
	res, err := New().Sync(context.Background(), makeSource(newTestConfigForTHGShape(srv.URL)), fw)
	if err != nil {
		t.Fatalf("Sync should not have returned fatal error: %v", err)
	}
	if res.AssetsRejected != 1 {
		t.Fatalf("AssetsRejected = %d, want 1", res.AssetsRejected)
	}
	if len(res.Errors) != 1 || res.Errors[0].Detail == "" {
		t.Fatalf("expected one SyncError with detail, got %#v", res.Errors)
	}
}

// newTestConfigForTHGShape is the explicit, correctly-typed config
// builder for THG-style upstream tests. Avoids the string alias
// hack from the (deprecated) newTestConfig wrapper at the top of
// this file.
func newTestConfigForTHGShape(baseURL string) json.RawMessage {
	raw := ExampleConfigTHGHub()
	var cfg Config
	_ = json.Unmarshal(raw, &cfg)
	cfg.BaseURL = baseURL
	cfg.Pagination.MaxPages = 10
	out, _ := json.Marshal(cfg)
	return out
}

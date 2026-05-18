package csv

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// captureWriter records every asset the ingestor produces so tests
// can assert without involving the store.
type captureWriter struct {
	mu     sync.Mutex
	assets []*assets.Asset
	errOn  int // 1-based row to fail on, 0 = never
}

func (c *captureWriter) Write(_ context.Context, a *assets.Asset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.assets = append(c.assets, a)
	if c.errOn > 0 && len(c.assets) == c.errOn {
		return errors.New("simulated write failure")
	}
	return nil
}

func makeSource(body string, mapCfg ColumnMap, typ assets.AssetType, tagCol string) *sources.Source {
	cfg := Config{
		AssetType: typ,
		ColumnMap: mapCfg,
		TagColumn: tagCol,
		Body:      body,
	}
	b, _ := json.Marshal(cfg)
	return &sources.Source{
		ID:               1,
		OrgID:            7,
		Type:             sources.SourceCSV,
		Label:            "test csv",
		ConnectionConfig: b,
	}
}

func TestCSV_HappyPath(t *testing.T) {
	body := `external_id,title,description,tags
SKU-1,Custom Cat Tee,Premium unisex,"cat,tee,unisex"
SKU-2,Cat Mug,Ceramic 11oz,"cat,mug"`

	src := makeSource(body, ColumnMap{}, assets.AssetPODProduct, "tags")
	w := &captureWriter{}
	res, err := New().Sync(context.Background(), src, w)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.AssetsSeen != 2 {
		t.Errorf("AssetsSeen: got %d want 2", res.AssetsSeen)
	}
	if res.AssetsRejected != 0 {
		t.Errorf("AssetsRejected: got %d want 0", res.AssetsRejected)
	}
	if len(w.assets) != 2 {
		t.Fatalf("captured %d assets, want 2", len(w.assets))
	}
	first := w.assets[0]
	if first.ExternalID != "SKU-1" {
		t.Errorf("external_id passthrough: got %q want SKU-1", first.ExternalID)
	}
	if first.Title != "Custom Cat Tee" {
		t.Errorf("title: got %q", first.Title)
	}
	if len(first.Tags) != 3 {
		t.Errorf("tags: got %v want 3 entries", first.Tags)
	}
}

func TestCSV_ColumnMappingRenamesHeaders(t *testing.T) {
	body := `Product ID,Product Name,Product Description,Wholesale Price
GID-001,Heavyweight Tee,6.1oz ring-spun cotton,$18.50`

	src := makeSource(body, ColumnMap{
		ExternalID:  "Product ID",
		Title:       "Product Name",
		Description: "Product Description",
	}, assets.AssetPODProduct, "")
	w := &captureWriter{}
	if _, err := New().Sync(context.Background(), src, w); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(w.assets) != 1 {
		t.Fatalf("got %d assets, want 1", len(w.assets))
	}
	a := w.assets[0]
	if a.ExternalID != "GID-001" {
		t.Errorf("mapped external_id: got %q", a.ExternalID)
	}
	if a.Title != "Heavyweight Tee" {
		t.Errorf("mapped title: got %q", a.Title)
	}
	// Wholesale Price went into payload because it wasn't mapped.
	var payload map[string]string
	if err := json.Unmarshal(a.Payload, &payload); err != nil {
		t.Fatalf("payload not json: %v", err)
	}
	if got := payload["wholesale price"]; got != "$18.50" {
		t.Errorf("unmapped column should be in payload; got %q for 'wholesale price'", got)
	}
}

func TestCSV_FingerprintForMissingExternalID(t *testing.T) {
	// No external_id column → ingestor falls back to ContentFingerprint.
	body := `title,description,tags
Generic Tee,Cotton,plain`

	src := makeSource(body, ColumnMap{}, assets.AssetPODProduct, "tags")
	w := &captureWriter{}
	if _, err := New().Sync(context.Background(), src, w); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(w.assets) != 1 {
		t.Fatalf("got %d assets", len(w.assets))
	}
	if w.assets[0].ExternalID == "" {
		t.Error("ExternalID should be a content fingerprint, got empty")
	}

	// Re-running with identical body produces the same fingerprint
	// (idempotent ingest depends on this).
	w2 := &captureWriter{}
	if _, err := New().Sync(context.Background(), src, w2); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	if w.assets[0].ExternalID != w2.assets[0].ExternalID {
		t.Errorf("fingerprint not stable across re-syncs: %q vs %q",
			w.assets[0].ExternalID, w2.assets[0].ExternalID)
	}
}

func TestCSV_BlankTitleRejectedPerRow(t *testing.T) {
	body := `title,description
Cat Tee,fine
,blank title here
Dog Mug,also fine`

	src := makeSource(body, ColumnMap{}, assets.AssetPODProduct, "")
	w := &captureWriter{}
	res, err := New().Sync(context.Background(), src, w)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.AssetsSeen != 2 {
		t.Errorf("AssetsSeen: got %d want 2", res.AssetsSeen)
	}
	if res.AssetsRejected != 1 {
		t.Errorf("AssetsRejected: got %d want 1", res.AssetsRejected)
	}
	if len(res.Errors) != 1 {
		t.Errorf("Errors: got %d want 1", len(res.Errors))
	}
	if !strings.Contains(res.Errors[0].Reason, "blank") &&
		!strings.Contains(res.Errors[0].Reason, "title") {
		t.Errorf("error should mention blank title; got %q", res.Errors[0].Reason)
	}
}

func TestCSV_PermanentErrorWhenAssetTypeUnknown(t *testing.T) {
	body := `title
x`
	src := makeSource(body, ColumnMap{}, "totally-bogus-type", "")
	_, err := New().Sync(context.Background(), src, &captureWriter{})
	if err == nil {
		t.Fatal("expected permanent error for unknown asset_type")
	}
	if ingestion.IsRecoverable(err) {
		t.Errorf("config errors must be permanent, not recoverable; got %v", err)
	}
}

func TestCSV_WriterErrorIsRecoverable(t *testing.T) {
	body := `title
ok`
	src := makeSource(body, ColumnMap{}, assets.AssetPODProduct, "")
	w := &captureWriter{errOn: 1}
	_, err := New().Sync(context.Background(), src, w)
	if err == nil {
		t.Fatal("expected error from failing writer")
	}
	if !ingestion.IsRecoverable(err) {
		t.Errorf("writer failures should be recoverable; got %v", err)
	}
}

func TestCSV_ContextCancellation(t *testing.T) {
	body := `title
a
b
c`
	src := makeSource(body, ColumnMap{}, assets.AssetPODProduct, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := New().Sync(ctx, src, &captureWriter{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled; got %v", err)
	}
}

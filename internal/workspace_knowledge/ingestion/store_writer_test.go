package ingestion

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// fakeAssetStore captures the last Upsert call so tests can assert on
// what the writer forwarded, without needing a real database.
type fakeAssetStore struct {
	got *assets.Asset
	err error
}

func (f *fakeAssetStore) UpsertKnowledgeAsset(_ context.Context, a *assets.Asset) (*assets.Asset, error) {
	clone := *a
	f.got = &clone
	if f.err != nil {
		return nil, f.err
	}
	return &clone, nil
}

func TestStoreAssetWriter_ForcesBinding(t *testing.T) {
	store := &fakeAssetStore{}
	src := &sources.Source{ID: 42, OrgID: 7}
	w := NewStoreAssetWriter(store, src)

	a := &assets.Asset{
		Type:  assets.AssetPODProduct,
		Title: "test",
		// OrgID + SourceID intentionally left zero — the writer must fill them.
	}
	if err := w.Write(context.Background(), a); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if store.got.OrgID != 7 {
		t.Errorf("OrgID was not forced; got %d want 7", store.got.OrgID)
	}
	if store.got.SourceID != 42 {
		t.Errorf("SourceID was not forced; got %d want 42", store.got.SourceID)
	}
}

func TestStoreAssetWriter_RejectsForeignOrgID(t *testing.T) {
	store := &fakeAssetStore{}
	src := &sources.Source{ID: 42, OrgID: 7}
	w := NewStoreAssetWriter(store, src)

	// Ingestor tries to smuggle into a different tenant by lying about OrgID.
	a := &assets.Asset{OrgID: 99, Title: "hostile"}
	err := w.Write(context.Background(), a)
	if !errors.Is(err, ErrCrossTenantWrite) {
		t.Errorf("expected ErrCrossTenantWrite for foreign OrgID; got %v", err)
	}
	if store.got != nil {
		t.Error("hostile write reached the store")
	}
}

func TestStoreAssetWriter_RejectsForeignSourceID(t *testing.T) {
	store := &fakeAssetStore{}
	src := &sources.Source{ID: 42, OrgID: 7}
	w := NewStoreAssetWriter(store, src)

	a := &assets.Asset{SourceID: 99, Title: "wrong source"}
	err := w.Write(context.Background(), a)
	if !errors.Is(err, ErrCrossTenantWrite) {
		t.Errorf("expected ErrCrossTenantWrite for foreign SourceID; got %v", err)
	}
}

func TestStoreAssetWriter_RejectsUnboundWriter(t *testing.T) {
	store := &fakeAssetStore{}
	// Empty source — neither OrgID nor ID set.
	w := NewStoreAssetWriter(store, &sources.Source{})

	a := &assets.Asset{Title: "test"}
	err := w.Write(context.Background(), a)
	if !errors.Is(err, ErrCrossTenantWrite) {
		t.Errorf("unbound writer must reject every Write; got %v", err)
	}
}

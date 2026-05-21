package ingestion

import (
	"context"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// AssetStore is the narrow subset of *knowledge.Store an ingestor writer
// needs. Defined here (not on the store side) so this package compiles
// without importing internal/store — keeps the dependency arrow
// pointing the right way: store → workspace_knowledge, never reverse.
//
// The production wiring uses *knowledge.Store directly because it
// satisfies this interface; tests pass a fake.
type AssetStore interface {
	UpsertAsset(ctx context.Context, a *assets.Asset) (*assets.Asset, error)
}

// NewStoreAssetWriter returns an AssetWriter bound to one specific
// (org, source) pair. Ingestors receive a writer from the dispatcher;
// they never construct one themselves. This is what makes Invariant
// #1 (tenant isolation) impossible to violate from an ingestor — the
// writer's binding is set by trusted code, and every Write call is
// validated against it.
//
// If src.OrgID == 0 or src.ID == 0, the writer rejects every Write
// call. There is no "global" writer.
func NewStoreAssetWriter(store AssetStore, src *sources.Source) AssetWriter {
	return &storeAssetWriter{store: store, orgID: src.OrgID, sourceID: src.ID}
}

type storeAssetWriter struct {
	store    AssetStore
	orgID    int64
	sourceID int64
}

func (w *storeAssetWriter) Write(ctx context.Context, a *assets.Asset) error {
	if w.orgID <= 0 || w.sourceID <= 0 {
		return ErrCrossTenantWrite
	}
	if a == nil {
		return ErrCrossTenantWrite
	}
	// Force the writer's binding onto the asset. Ingestors do not get
	// to choose; if they passed a different OrgID/SourceID we reject —
	// silently coercing would mask a bug.
	if a.OrgID != 0 && a.OrgID != w.orgID {
		return ErrCrossTenantWrite
	}
	if a.SourceID != 0 && a.SourceID != w.sourceID {
		return ErrCrossTenantWrite
	}
	a.OrgID = w.orgID
	a.SourceID = w.sourceID
	_, err := w.store.UpsertAsset(ctx, a)
	return err
}

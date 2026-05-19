package products

import (
	"context"
	"fmt"

	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
)

// Adapter is the contract for ingestors that emit product-catalog
// assets. It is a type alias of [ingestion.Ingestor]: there is no
// parallel dispatcher for product catalogs — adapters register into
// the existing [ingestion.Registry] under their own
// [sources.SourceType] (e.g. "shopify", "rest_json_catalog",
// "woocommerce") and the dispatcher routes by source type as usual.
//
// The named alias documents intent in adapter package signatures and
// lets the compiler enforce "this Ingestor specifically emits product
// assets" via a Writer parameter (see [NewWriter]). Adapters that emit
// FAQ or playbook assets implement Ingestor directly and never touch
// this package.
//
// New adapters MUST:
//
//   - Declare their own [sources.SourceType] constant.
//   - Use [NewWriter] to wrap the [ingestion.AssetWriter] they receive.
//   - Call [CanonicalProduct.Normalize] and [CanonicalProduct.Validate]
//     before [Writer.Write]. Validate errors are not fatal: surface
//     them in [ingestion.SyncResult.Errors] and continue with the next
//     row.
//   - Set [CanonicalProduct.ExtractorVersion] to a stable identifier
//     for their wire-format mapping (e.g. "shopify/v3"). Bump when
//     the mapping changes so historical assets can be re-extracted.
type Adapter = ingestion.Ingestor

// Writer is the product-specific surface adapters use. It wraps the
// generic [ingestion.AssetWriter] received from the dispatcher and
// handles the CanonicalProduct → assets.Asset mapping internally, so
// adapter code never constructs an Asset directly.
//
// Centralising the mapping has two practical wins:
//
//  1. The mapping rule (Title fallbacks, Tag union, Payload schema)
//     lives in exactly one place — change [ToAsset] and every adapter
//     benefits without a touch.
//
//  2. The writer enforces "you normalised + validated before writing"
//     by calling both at write time. An adapter that forgets gets a
//     clear error from this layer, not a confusing failure deeper in
//     the asset writer.
type Writer struct {
	inner ingestion.AssetWriter
}

// NewWriter wraps an [ingestion.AssetWriter] for product-catalog use.
// Adapters receive the inner writer from the dispatcher (via their
// [Adapter.Sync] argument) and call NewWriter once at the start of
// the sync to get the product-typed write surface.
//
// Returns nil if inner is nil — the caller should treat that as a
// programmer error and refuse to proceed.
func NewWriter(inner ingestion.AssetWriter) *Writer {
	if inner == nil {
		return nil
	}
	return &Writer{inner: inner}
}

// Write normalises, validates, and persists one canonical product.
//
// On Validate failure the error is returned directly. Adapters
// SHOULD catch it, record an [ingestion.SyncError], and continue
// with the next item — one bad row should never abort the whole
// sync. The dispatcher's contract for [ingestion.SyncResult] reflects
// this (AssetsRejected counts validation failures separately from
// hard errors).
//
// Write does not call ingestion.WrapRecoverable/WrapPermanent —
// validation errors are permanent (the row is malformed in the
// upstream) and the AssetWriter's own errors are bubbled up
// unchanged so the adapter can classify them with its own knowledge
// of the upstream system.
func (w *Writer) Write(ctx context.Context, p *CanonicalProduct) error {
	if w == nil {
		return fmt.Errorf("products: nil writer")
	}
	if p == nil {
		return fmt.Errorf("products: nil product")
	}
	p.Normalize()
	if err := p.Validate(); err != nil {
		return err
	}
	asset, err := ToAsset(p)
	if err != nil {
		return err
	}
	return w.inner.Write(ctx, asset)
}

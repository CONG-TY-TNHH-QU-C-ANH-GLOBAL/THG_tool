// Package shopify is the Shopify Admin API ingestor stub for the
// Workspace Knowledge OS.
//
// STATUS: stub. The contract and surface are stable; the API client
// is not implemented. Sync returns a permanent error so the
// dispatcher marks the source as "error" with a clear message —
// operators see "Shopify ingestor not yet implemented" in the UI
// rather than a silent failure.
//
// To finish this connector in Phase B.2:
//   1. Add an Admin API client (recommendation: github.com/bold-commerce/go-shopify/v4).
//      Auth: per-source OAuth token from src.ConnectionConfig.shop_token.
//      Domain:  src.ConnectionConfig.shop_domain (e.g. "thg.myshopify.com").
//   2. Page through GET /admin/api/2024-01/products.json?limit=250&page_info=...
//      and convert each ProductVariant into one [assets.Asset]:
//        - ExternalID = strconv.FormatInt(product.ID, 10)  (Shopify product ID)
//        - Type       = assets.AssetPODProduct
//        - Title      = product.Title
//        - Description = stripHTML(product.BodyHTML)
//        - Tags       = strings.Split(product.Tags, ",")
//        - Payload    = JSON of {variants[], images[], price_range, vendor}
//   3. Handle the standard Shopify error shapes:
//        - 401 / invalid_token  → ingestion.WrapPermanent + Health.Status = needs_auth
//        - 429 rate limit       → ingestion.WrapRecoverable
//        - 5xx                  → ingestion.WrapRecoverable
//   4. Test with a fake admin API server (httptest.NewServer + a fixture set).
//
// The Sync signature itself does NOT change. Phase B.2 only fills in
// the body of this file.
package shopify

import (
	"context"
	"errors"

	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

type Ingestor struct{}

func New() *Ingestor                          { return &Ingestor{} }
func (i *Ingestor) Type() sources.SourceType  { return sources.SourceShopify }

func (i *Ingestor) Sync(_ context.Context, _ *sources.Source, _ ingestion.AssetWriter) (ingestion.SyncResult, error) {
	return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New(
		"shopify ingestor not yet implemented — Phase B.2 (specs/WORKSPACE_KNOWLEDGE_OS.md §7)",
	))
}

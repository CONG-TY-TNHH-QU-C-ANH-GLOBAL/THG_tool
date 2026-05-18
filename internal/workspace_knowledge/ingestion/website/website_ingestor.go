// Package website is the website-crawler ingestor stub.
//
// STATUS: stub.
//
// Finishing in Phase B.5:
//   1. Reuse the existing crawler at internal/browser or
//      internal/jobhandlers/facebook_crawl — but constrained to one
//      hostname per source. src.ConnectionConfig.start_url +
//      src.ConnectionConfig.allow_hosts (default = host of start_url).
//   2. Convert each crawled page into one Asset of type
//      [assets.AssetFAQ] (or [assets.AssetSalesPlaybook] when the
//      page looks long-form). The classifier is downstream — at
//      ingest time we just record what we crawled.
//   3. ExternalID = sha1(canonicalized URL). Stable across re-crawls.
//   4. Operators rarely need realtime sync for marketing pages —
//      enforce src.SyncPolicy = sources.SyncDaily for this type
//      unless the operator explicitly overrode it.
//
// NOT a search engine: the website ingestor does NOT crawl every
// page on a domain. It crawls the seed URL and one or two hops of
// internal-link discovery, then stops. Large-site indexing is a
// product question, not an ingestor question.
package website

import (
	"context"
	"errors"

	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

type Ingestor struct{}

func New() *Ingestor                         { return &Ingestor{} }
func (i *Ingestor) Type() sources.SourceType { return sources.SourceWebsite }

func (i *Ingestor) Sync(_ context.Context, _ *sources.Source, _ ingestion.AssetWriter) (ingestion.SyncResult, error) {
	return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New(
		"website ingestor not yet implemented — Phase B.5",
	))
}

// Package notion is the Notion API ingestor stub.
//
// STATUS: stub.
//
// Finishing in Phase B.4:
//   1. Use github.com/jomei/notionapi (or roll a thin client — Notion
//      API surface used is small: GET /databases/{id}/query).
//   2. Auth: per-source OAuth or internal-integration token from
//      src.ConnectionConfig.token. Operators add via the Notion-
//      integration-token UI flow.
//   3. src.ConnectionConfig.database_id selects the database; each
//      page becomes one Asset with type usually [assets.AssetSalesPlaybook].
//      Map a "Title" property → Asset.Title; flatten the page body
//      blocks into Description.
//   4. ExternalID = page.ID (Notion's UUID is stable across edits).
//   5. Token expiry → ingestion.WrapPermanent so the dispatcher
//      surfaces needs_auth.
package notion

import (
	"context"
	"errors"

	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

type Ingestor struct{}

func New() *Ingestor                         { return &Ingestor{} }
func (i *Ingestor) Type() sources.SourceType { return sources.SourceNotion }

func (i *Ingestor) Sync(_ context.Context, _ *sources.Source, _ ingestion.AssetWriter) (ingestion.SyncResult, error) {
	return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New(
		"notion ingestor not yet implemented — Phase B.4",
	))
}

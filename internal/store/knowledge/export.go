package knowledge

import (
	"context"
	"fmt"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Knowledge OS export read model — the source side of the THG VectorHub
// derived-index sync (specs/EXPORT_ENDPOINT_CONTRACT.md over in the
// vectorhub repo). It is deliberately a separate method from
// ListAssetsForOrg: that one powers the operator Product Explorer with
// rich filters + offset paging, while this one is a single-purpose,
// keyset-paged, all-states export. Keeping them apart avoids overloading
// the hot list path with cursor logic.

// ExportCursor is a keyset position. A page returns rows strictly after
// (UpdatedAfter, AfterID) in (updated_at, id) ascending order. The zero
// value starts from the beginning of the org's assets.
type ExportCursor struct {
	UpdatedAfter time.Time // exclusive lower bound on updated_at
	AfterID      int64     // tiebreak among rows sharing the same second
}

// sqliteTimeLayout is the fixed-width, lexicographically-monotonic format
// SQLite's CURRENT_TIMESTAMP writes. Comparing the formatted string is
// exact at second resolution, so keyset paging needs no datetime() parse
// and no dialect-specific casting.
const sqliteTimeLayout = "2006-01-02 15:04:05"

// exportMaxLimit caps a single page. The connector defaults to 500; a
// caller asking for more is clamped rather than rejected.
const exportMaxLimit = 500

// ExportAssetsForOrg returns up to limit assets owned by orgID, ordered by
// (updated_at, id) ascending and strictly after cur. It returns assets in
// EVERY state (approved, pending, hidden) on purpose: the derived index
// mirrors source state downstream and must NOT filter to approved-only —
// each consumer attaches its own policy gate. Read-only; org isolation is
// implicit on the receiver, exactly like ListAssetsForOrg.
func (s *Store) ExportAssetsForOrg(ctx context.Context, orgID int64, cur ExportCursor, limit int) ([]*assets.Asset, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge: org_id is required")
	}
	if limit <= 0 || limit > exportMaxLimit {
		limit = exportMaxLimit
	}
	after := cur.UpdatedAfter.UTC().Format(sqliteTimeLayout)
	q := assetSelect + `
		WHERE org_id = ?
		  AND (updated_at > ? OR (updated_at = ? AND id > ?))
		ORDER BY updated_at ASC, id ASC
		LIMIT ?`
	rows, err := s.queryContext(ctx, q, orgID, after, after, cur.AfterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*assets.Asset, 0, limit)
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

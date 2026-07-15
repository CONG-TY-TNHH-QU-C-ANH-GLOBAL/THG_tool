package export

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// exportItem is the wire row the vectorhub thgtool connector decodes.
// external_id is the asset's stable numeric id as a string: unique per
// org and never empty, so the derived index can key on it (platform
// invariant I1). The source's own external_id (which may be empty or
// non-unique across sources) is preserved in metadata for traceability.
type exportItem struct {
	ExternalID  string         `json:"external_id"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Tags        []string       `json:"tags"`
	Metadata    map[string]any `json:"metadata"`
	State       string         `json:"state"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// exportResponse mirrors the JSON shape the connector expects: a page of
// items plus the keyset cursor for the next page and a done flag.
type exportResponse struct {
	Items            []exportItem `json:"items"`
	NextUpdatedAfter time.Time    `json:"next_updated_after"`
	NextAfterID      string       `json:"next_after_id"`
	Done             bool         `json:"done"`
}

const defaultLimit = 500

// exportAssets serves one keyset page for the configured org.
func (h *handler) exportAssets(c *fiber.Ctx) error {
	cur, err := parseCursor(c)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	limit := parseLimit(c.Query("limit"))

	rows, err := h.deps.DB.Knowledge().ExportAssetsForOrg(c.Context(), h.deps.OrgID, cur, limit)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "export failed")
	}
	return c.JSON(buildResponse(rows, limit))
}

// parseCursor reads the keyset position from the query string. Absent
// params yield the zero cursor (start from the beginning).
func parseCursor(c *fiber.Ctx) (knowledge.ExportCursor, error) {
	var cur knowledge.ExportCursor
	if ua := c.Query("updated_after"); ua != "" {
		t, err := time.Parse(time.RFC3339, ua)
		if err != nil {
			return cur, fmt.Errorf("invalid updated_after: want RFC3339")
		}
		cur.UpdatedAfter = t
	}
	if ai := c.Query("after_id"); ai != "" {
		id, err := strconv.ParseInt(ai, 10, 64)
		if err != nil {
			return cur, fmt.Errorf("invalid after_id: want integer")
		}
		cur.AfterID = id
	}
	return cur, nil
}

// parseLimit clamps the requested page size into (0, defaultLimit].
func parseLimit(raw string) int {
	if raw == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > defaultLimit {
		return defaultLimit
	}
	return n
}

// buildResponse maps store rows to wire items and derives the next cursor
// from the last row. done is true when the page was not full, i.e. the
// source has no more rows after this page.
func buildResponse(rows []*assets.Asset, limit int) exportResponse {
	resp := exportResponse{
		Items: make([]exportItem, 0, len(rows)),
		Done:  len(rows) < limit,
	}
	for _, a := range rows {
		resp.Items = append(resp.Items, toExportItem(a))
	}
	if n := len(resp.Items); n > 0 {
		last := resp.Items[n-1]
		resp.NextAfterID = last.ExternalID
		resp.NextUpdatedAfter = last.UpdatedAt
	}
	return resp
}

// toExportItem projects one asset onto the export wire shape.
func toExportItem(a *assets.Asset) exportItem {
	meta := map[string]any{
		"source_id": a.SourceID,
		"pinned":    a.Pinned,
		"boost":     a.Boost,
	}
	if a.ExternalID != "" {
		meta["source_external_id"] = a.ExternalID
	}
	tags := a.Tags
	if tags == nil {
		tags = []string{}
	}
	return exportItem{
		ExternalID:  strconv.FormatInt(a.ID, 10),
		Type:        string(a.Type),
		Title:       a.Title,
		Description: a.Description,
		Tags:        tags,
		Metadata:    meta,
		State:       string(a.State),
		UpdatedAt:   a.UpdatedAt,
	}
}

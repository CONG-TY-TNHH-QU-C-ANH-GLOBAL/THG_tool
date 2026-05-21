package knowledge

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// listSources returns every knowledge source owned by the caller's
// org. Org scoping is enforced by the store layer; this handler
// never accepts an org_id query parameter.
//
// Optional filters:
//   - ?type=rest_json (repeatable) narrows to specific source types
//   - ?health=healthy (repeatable) narrows to specific health states
func (h *handler) listSources(c *fiber.Ctx) error {
	orgID, ok := c.Locals("org_id").(int64)
	if !ok || orgID <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "org context required"})
	}

	filter := sources.ListFilter{}
	if raw := c.Query("type"); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			filter.Types = append(filter.Types, sources.SourceType(strings.TrimSpace(t)))
		}
	}
	if raw := c.Query("health"); raw != "" {
		for _, hs := range strings.Split(raw, ",") {
			filter.Health = append(filter.Health, sources.HealthStatus(strings.TrimSpace(hs)))
		}
	}

	list, err := h.deps.DB.Knowledge().ListSourcesForOrg(c.Context(), orgID, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"sources": list,
		"count":   len(list),
	})
}

// createSourceBody is the wire shape for POST /knowledge/sources.
// The connection_config is opaque from this handler's perspective â€”
// the adapter behind the chosen Type validates it on first sync.
type createSourceBody struct {
	Type             string          `json:"type"`
	Label            string          `json:"label"`
	ConnectionConfig json.RawMessage `json:"connection_config"`
	SyncPolicy       string          `json:"sync_policy"`
}

func (h *handler) createSource(c *fiber.Ctx) error {
	orgID, ok := c.Locals("org_id").(int64)
	if !ok || orgID <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "org context required"})
	}
	var body createSourceBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	srcType := sources.SourceType(strings.TrimSpace(body.Type))
	if !srcType.IsKnown() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "unknown source type: " + body.Type,
		})
	}

	syncPolicy := sources.SyncPolicy(strings.TrimSpace(body.SyncPolicy))
	if !syncPolicy.IsKnown() {
		syncPolicy = sources.SyncManual
	}

	src := &sources.Source{
		OrgID:            orgID,
		Type:             srcType,
		Label:            strings.TrimSpace(body.Label),
		ConnectionConfig: body.ConnectionConfig,
		SyncPolicy:       syncPolicy,
	}
	if err := src.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	created, err := h.deps.DB.Knowledge().UpsertSource(c.Context(), src)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

// updateSourceBody is a partial-update body â€” every field is optional;
// the handler only touches fields the caller sent. ConnectionConfig
// is opaque-replaced as a whole (no JSON-Patch semantics) because
// adapter-specific merge logic does not belong in HTTP.
type updateSourceBody struct {
	Label            *string         `json:"label,omitempty"`
	ConnectionConfig json.RawMessage `json:"connection_config,omitempty"`
	SyncPolicy       *string         `json:"sync_policy,omitempty"`
}

func (h *handler) updateSource(c *fiber.Ctx) error {
	orgID, ok := c.Locals("org_id").(int64)
	if !ok || orgID <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "org context required"})
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid source id"})
	}

	existing, err := h.deps.DB.Knowledge().GetSource(c.Context(), id, orgID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "source not found"})
	}

	var body updateSourceBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if body.Label != nil {
		existing.Label = strings.TrimSpace(*body.Label)
	}
	if len(body.ConnectionConfig) > 0 {
		existing.ConnectionConfig = body.ConnectionConfig
	}
	if body.SyncPolicy != nil {
		sp := sources.SyncPolicy(strings.TrimSpace(*body.SyncPolicy))
		if sp.IsKnown() {
			existing.SyncPolicy = sp
		}
	}
	if err := existing.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	updated, err := h.deps.DB.Knowledge().UpsertSource(c.Context(), existing)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(updated)
}

func (h *handler) deleteSource(c *fiber.Ctx) error {
	orgID, ok := c.Locals("org_id").(int64)
	if !ok || orgID <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "org context required"})
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid source id"})
	}
	deleted, err := h.deps.DB.Knowledge().DeleteSourceForOrg(c.Context(), id, orgID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"ok":             true,
		"assets_deleted": deleted,
	})
}

// syncSource triggers a synchronous sync via the dispatcher. The
// adapter behind the source's Type runs end-to-end (paginated fetch,
// per-item extract, asset upsert) and the handler returns the
// SyncResult inline. Long-running syncs should move to a background
// queue in a later PR â€” v1 holds the request open so operators see
// the outcome in one click.
//
// Errors from the adapter are translated to HTTP:
//   - permanent â†’ 4xx body with detail
//   - recoverable â†’ 502 body with retry hint
//   - context cancel â†’ 504
func (h *handler) syncSource(c *fiber.Ctx) error {
	orgID, ok := c.Locals("org_id").(int64)
	if !ok || orgID <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "org context required"})
	}
	if h.deps.Dispatcher == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ingest dispatcher not configured",
		})
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid source id"})
	}
	src, err := h.deps.DB.Knowledge().GetSource(c.Context(), id, orgID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "source not found"})
	}

	res, syncErr := h.deps.Dispatcher.Run(c.Context(), src)

	// We always return the SyncResult â€” operators want to see partial
	// progress even on a failed sync (the dispatcher already wrote
	// successful rows to the store before the error surfaced).
	body := fiber.Map{"result": res}
	if syncErr != nil {
		body["error"] = syncErr.Error()
		// Recoverable â†’ 502 (upstream blip, retry later);
		// Permanent â†’ 422 (config/auth/schema problem, operator must act).
		// The dispatcher already persisted the source's health row
		// before returning, so the UI reflects the failure regardless.
		status := fiber.StatusBadGateway
		if !ingestion.IsRecoverable(syncErr) {
			status = fiber.StatusUnprocessableEntity
		}
		return c.Status(status).JSON(body)
	}
	return c.JSON(body)
}

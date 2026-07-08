package reel

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	reelsvc "github.com/thg/scraper/internal/services/reel"
)

// Transport-layer messages, factored out to avoid duplicated string literals.
const (
	errOrgRequired = "org context required"
	errInvalidReel = "invalid reel_id"
	errBadRequest  = "invalid request"
	errInternal    = "internal error"
)

// orgID returns the authenticated org from context, or 0 if absent.
func orgID(c *fiber.Ctx) int64 { id, _ := c.Locals("org_id").(int64); return id }

// reelIDParam parses the :reel_id path param. Returns 0 on any malformed value.
func reelIDParam(c *fiber.Ctx) int64 {
	id, err := strconv.ParseInt(c.Params("reel_id"), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// mapErr translates a service/store error to a stable HTTP status. Unknown
// errors return a generic 500 message — internal error strings are never
// leaked to the client.
func mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, reelsvc.ErrReelNotFound),
		errors.Is(err, reelsvc.ErrNoScript),
		errors.Is(err, sql.ErrNoRows):
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, reelsvc.ErrScriptNotApproved):
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(500).JSON(fiber.Map{"error": errInternal})
	}
}

func (h *handler) createDraft(c *fiber.Ctx) error {
	org := orgID(c)
	if org <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": errOrgRequired})
	}
	createdBy, _ := c.Locals("user_id").(int64)
	var body struct {
		Title string `json:"title"`
		Brief string `json:"brief"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": errBadRequest})
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "title is required"})
	}
	id, err := h.svc.CreateDraft(c.Context(), org, createdBy, body.Title, strings.TrimSpace(body.Brief))
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(201).JSON(fiber.Map{"reel_id": id})
}

func (h *handler) generateScript(c *fiber.Ctx) error {
	org := orgID(c)
	if org <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": errOrgRequired})
	}
	reelID := reelIDParam(c)
	if reelID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": errInvalidReel})
	}
	script, err := h.svc.GenerateScript(c.Context(), org, reelID)
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(201).JSON(fiber.Map{
		"reel_id":   script.ReelID,
		"script_id": script.ID,
		"version":   script.Version,
	})
}

func (h *handler) approveLatest(c *fiber.Ctx) error {
	org := orgID(c)
	if org <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": errOrgRequired})
	}
	reelID := reelIDParam(c)
	if reelID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": errInvalidReel})
	}
	if err := h.svc.ApproveLatestScript(c.Context(), org, reelID); err != nil {
		return mapErr(c, err)
	}
	return c.JSON(fiber.Map{"status": reelsvc.StatusApproved})
}

func (h *handler) renderFake(c *fiber.Ctx) error {
	org := orgID(c)
	if org <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": errOrgRequired})
	}
	reelID := reelIDParam(c)
	if reelID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": errInvalidReel})
	}
	if err := h.svc.RenderFake(c.Context(), org, reelID); err != nil {
		return mapErr(c, err)
	}
	return c.JSON(fiber.Map{"status": reelsvc.StatusDone})
}

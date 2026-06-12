// Package contactprofile serves staff/sales contact profiles (PR-5):
// each member edits their OWN contact identity; admins can view/manage
// every profile in the workspace. Company identity stays a separate
// surface (org settings) — this is the per-person contact line.
package contactprofile

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

type Deps struct {
	DB *store.Store
}

type handler struct{ deps Deps }

// Routes registers under the tenant-authenticated group.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := &handler{deps: deps}
	group.Get("/me/contact-profile", h.getOwn)
	group.Put("/me/contact-profile", h.putOwn)
	group.Get("/org/contact-profiles", adminOnly, h.list)
	group.Put("/org/contact-profiles/:userID", adminOnly, h.putForUser)
}

func (h *handler) getOwn(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	p, err := h.deps.DB.GetStaffContactProfile(orgID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if p == nil {
		p = &models.StaffContactProfile{UserID: userID, OrgID: orgID, Active: true, Visibility: "team"}
	}
	return c.JSON(fiber.Map{"profile": p, "contact_line": p.ContactLine()})
}

func (h *handler) putOwn(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	return h.save(c, orgID, userID)
}

func (h *handler) putForUser(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	targetID, err := strconv.ParseInt(c.Params("userID"), 10, 64)
	if err != nil || targetID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	// Admin may only manage profiles of members of THEIR org.
	target, err := h.deps.DB.GetUserByID(targetID)
	if err != nil || target == nil || target.OrgID != orgID {
		return c.Status(404).JSON(fiber.Map{"error": "user not found in workspace"})
	}
	return h.save(c, orgID, targetID)
}

func (h *handler) save(c *fiber.Ctx, orgID, userID int64) error {
	var req models.StaffContactProfile
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	req.UserID = userID
	req.OrgID = orgID // never trust body for tenancy
	req.Visibility = strings.TrimSpace(req.Visibility)
	if req.Visibility == "" {
		req.Visibility = "team"
	}
	if err := h.deps.DB.UpsertStaffContactProfile(&req); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	actorID, _ := c.Locals("user_id").(int64)
	h.deps.DB.InsertAuditLog(actorID, "staff_contact_profile_updated", c.IP(),
		`{"target_user_id":`+strconv.FormatInt(userID, 10)+`}`)
	return c.JSON(fiber.Map{"status": "saved", "contact_line": req.ContactLine()})
}

func (h *handler) list(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	profiles, err := h.deps.DB.ListStaffContactProfiles(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"profiles": profiles, "count": len(profiles)})
}

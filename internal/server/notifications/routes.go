// Package notifications serves the in-app notification bell
// (SaaS UX Hardening PR-1). Read + mark-read only — writes happen at
// the emitting features (invite create/accept, extension gate).
package notifications

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

type Deps struct {
	DB *store.Store
}

type handler struct {
	deps Deps
}

// Routes registers /notifications under the JWT-authenticated group.
func Routes(group fiber.Router, deps Deps) {
	h := &handler{deps: deps}
	group.Get("/notifications", h.list)
	group.Post("/notifications/read-all", h.markAllRead)
	group.Post("/notifications/:id/read", h.markRead)
}

func callerScope(c *fiber.Ctx) (orgID, userID int64, isAdmin bool) {
	orgID, _ = c.Locals("org_id").(int64)
	userID, _ = c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	r := models.UserRole(role)
	return orgID, userID, r == models.RoleAdmin || models.IsPlatformRole(r)
}

func (h *handler) list(c *fiber.Ctx) error {
	orgID, userID, isAdmin := callerScope(c)
	limit := c.QueryInt("limit", 50)
	items, err := h.deps.DB.ListNotificationsForUser(orgID, userID, isAdmin, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	unread, err := h.deps.DB.CountUnreadNotifications(orgID, userID, isAdmin)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"notifications": items, "unread": unread, "count": len(items)})
}

func (h *handler) markRead(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid notification id"})
	}
	orgID, userID, isAdmin := callerScope(c)
	if err := h.deps.DB.MarkNotificationRead(id, orgID, userID, isAdmin); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "notification not found"})
	}
	return c.JSON(fiber.Map{"status": "read"})
}

func (h *handler) markAllRead(c *fiber.Ctx) error {
	orgID, userID, isAdmin := callerScope(c)
	if err := h.deps.DB.MarkAllNotificationsRead(orgID, userID, isAdmin); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "read"})
}

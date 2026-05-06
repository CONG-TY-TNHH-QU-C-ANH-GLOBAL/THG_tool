package org

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
)

func (h *Handler) adminUpdateUser(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	var req struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		Active      *bool  `json:"active"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	user, err := h.deps.DB.GetUserByID(id)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	callerOrgID, _ := c.Locals("org_id").(int64)
	callerRole, _ := c.Locals("user_role").(string)
	callerIsPlatform := models.IsPlatformUser(callerOrgID, models.UserRole(callerRole))
	if !callerIsPlatform && user.OrgID != callerOrgID {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if !callerIsPlatform && models.IsPlatformRole(user.Role) {
		return c.Status(403).JSON(fiber.Map{"error": "cannot modify founder users"})
	}
	name := user.Name
	if req.Name != "" {
		name = req.Name
	}
	role := user.Role
	if req.Role == "admin" || req.Role == "sales" {
		role = models.UserRole(req.Role)
	}
	active := user.Active
	if req.Active != nil {
		active = *req.Active
	}
	if err := h.deps.DB.UpdateUser(id, name, role, active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	if req.NewPassword != "" {
		if err := auth.ValidatePasswordStrength(req.NewPassword); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "password hashing failed"})
		}
		h.deps.DB.UpdateUserPassword(id, hash)
		h.deps.DB.DeleteUserRefreshTokens(id)
	}
	adminID, _ := c.Locals("user_id").(int64)
	h.deps.DB.InsertAuditLog(adminID, "user_updated", c.IP(), fmt.Sprintf(`{"target_id":%d}`, id))
	return c.JSON(fiber.Map{"status": "updated"})
}

// adminDeleteUser handles DELETE /api/auth/users/:id â€” admin removes a user.
func (h *Handler) adminDeleteUser(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	adminID, _ := c.Locals("user_id").(int64)
	if id == adminID {
		return c.Status(400).JSON(fiber.Map{"error": "cannot delete your own account"})
	}
	user, err := h.deps.DB.GetUserByID(id)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	callerOrgID, _ := c.Locals("org_id").(int64)
	callerRole, _ := c.Locals("user_role").(string)
	callerIsPlatform := models.IsPlatformUser(callerOrgID, models.UserRole(callerRole))
	if !callerIsPlatform && user.OrgID != callerOrgID {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if !callerIsPlatform && models.IsPlatformRole(user.Role) {
		return c.Status(403).JSON(fiber.Map{"error": "cannot delete founder users"})
	}
	if err := h.deps.DB.DeleteUser(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "delete failed"})
	}
	h.deps.DB.InsertAuditLog(adminID, "user_deleted", c.IP(), fmt.Sprintf(`{"deleted_id":%d}`, id))
	return c.JSON(fiber.Map{"status": "deleted"})
}

// listUsers handles GET /api/auth/users â€” scoped to caller's org (superadmin sees all).
func (h *Handler) listUsers(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	users, err := h.deps.DB.ListUsers(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"users": users, "count": len(users)})
}

// getAuditLogs handles GET /api/auth/audit â€” admin views the security audit trail.
func (h *Handler) getAuditLogs(c *fiber.Ctx) error {
	limit := 100
	orgID, _ := c.Locals("org_id").(int64)
	role, _ := c.Locals("user_role").(string)
	var (
		logs []models.AuditLog
		err  error
	)
	if models.IsPlatformUser(orgID, models.UserRole(role)) {
		logs, err = h.deps.DB.GetAuditLogs(limit)
	} else {
		logs, err = h.deps.DB.GetAuditLogsByOrg(orgID, limit)
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"logs": logs, "count": len(logs)})
}

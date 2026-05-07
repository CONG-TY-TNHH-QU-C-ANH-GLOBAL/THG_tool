package autoflow

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// outboundModeAuto and outboundModeDraft are the only accepted values for the
// org-scoped outbound_mode policy. Any other value is normalized to draft so
// the default safety behavior (CLAUDE.md hard rule) is preserved.
const (
	outboundModeAuto  = "auto"
	outboundModeDraft = "draft"
)

func normalizeOutboundMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), outboundModeAuto) {
		return outboundModeAuto
	}
	return outboundModeDraft
}

// getOrgPolicy returns the org's automation policy flags. Read-allowed for any
// authenticated workspace member so the UI can render the current state.
func (h *Handler) getOrgPolicy(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	mode, _ := h.deps.DB.GetContext(orgContextKey(orgID, "outbound_mode"))
	return c.JSON(fiber.Map{
		"outbound_mode": normalizeOutboundMode(mode),
	})
}

// updateOrgPolicy writes the org's automation policy. Admin-only. Audit logged
// because flipping outbound_mode=auto bypasses the approval queue.
func (h *Handler) updateOrgPolicy(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	var body struct {
		OutboundMode string `json:"outbound_mode"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	mode := normalizeOutboundMode(body.OutboundMode)
	if err := h.deps.DB.SetContext(orgContextKey(orgID, "outbound_mode"), mode); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	userID, _ := c.Locals("user_id").(int64)
	h.deps.DB.InsertAuditLog(userID, "org_policy_updated", c.IP(),
		fmt.Sprintf(`{"outbound_mode":%q}`, mode))
	return c.JSON(fiber.Map{"ok": true, "outbound_mode": mode})
}

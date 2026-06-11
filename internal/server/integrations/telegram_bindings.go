package integrations

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/control"
)

// listBindings returns bindings for the org. Admins/platform owners see ALL bindings; a normal
// member sees only their own (enforced server-side, never trusting the client).
func (h *Handler) listBindings(c *fiber.Ctx) error {
	orgID, userID, role := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	var (
		bindings []telegram.Binding
		err      error
	)
	if canViewAllBindings(role) {
		bindings, err = h.deps.DB.Telegram().ListBindings(orgID)
	} else {
		bindings, err = h.deps.DB.Telegram().ListBindingsByUser(orgID, userID)
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "load bindings failed"})
	}
	return c.JSON(fiber.Map{"bindings": toBindingDTOs(bindings), "can_manage_all": canViewAllBindings(role)})
}

// bindingDTO is the wire shape for the UI: surfaces last_command_at (the store struct keeps it as
// sql.NullTime, json:"-"); chat_id is never exposed.
type bindingDTO struct {
	telegram.Binding
	LastCommandAt *time.Time `json:"last_command_at"`
}

func toBindingDTOs(bs []telegram.Binding) []bindingDTO {
	out := make([]bindingDTO, 0, len(bs))
	for _, b := range bs {
		d := bindingDTO{Binding: b}
		if b.LastCommandAt.Valid {
			t := b.LastCommandAt.Time
			d.LastCommandAt = &t
		}
		out = append(out, d)
	}
	return out
}

// revokeBinding revokes a binding. Admins/platform owners may revoke any binding in their org; a
// normal member may revoke only their own. Revocation is audited (never a hard delete).
func (h *Handler) revokeBinding(c *fiber.Ctx) error {
	orgID, userID, role := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	binding, err := h.deps.DB.Telegram().GetBinding(orgID, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "load binding failed"})
	}
	if binding == nil {
		return c.Status(404).JSON(fiber.Map{"error": "binding not found"})
	}
	if !canViewAllBindings(role) && binding.UserID != userID {
		return c.Status(403).JSON(fiber.Map{"error": "cannot revoke another user's binding"})
	}
	if err := h.deps.DB.Telegram().RevokeBinding(orgID, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "revoke failed"})
	}
	_ = h.deps.DB.Telegram().InsertAudit(orgID, userID, binding.TelegramUserID, control.AuditBindingRevoked, "ok", "")
	return c.JSON(fiber.Map{"revoked": true})
}

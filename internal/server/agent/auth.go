package agent

import (
	"log"

	"github.com/gofiber/fiber/v2"
	servermw "github.com/thg/scraper/internal/server/middleware"
)

// agentAuth middleware validates X-Agent-Token header.
func (h *Handler) agentAuth(c *fiber.Ctx) error {
	plain := c.Get("X-Agent-Token")
	if plain == "" {
		log.Printf("[AgentAuth] missing token path=%s ip=%s ua=%q", c.Path(), c.IP(), c.Get("User-Agent"))
		return c.Status(401).JSON(fiber.Map{"error": "missing X-Agent-Token"})
	}
	tok, err := h.db.Connectors().ValidateAgentToken(plain)
	if err != nil || tok == nil {
		log.Printf("[AgentAuth] rejected token path=%s ip=%s fp=%s err=%v", c.Path(), c.IP(), servermw.AgentTokenFingerprint(plain), err)
		return c.Status(401).JSON(fiber.Map{"error": "invalid or revoked agent token"})
	}
	c.Locals("agent_id", tok.ID)
	c.Locals("agent_org_id", tok.OrgID)
	c.Locals("agent_kind", tok.Kind)
	c.Locals("agent_name", tok.Name)
	c.Locals("agent_created_by", tok.CreatedBy)
	c.Locals("agent_assigned_account_id", tok.AssignedAccountID)
	c.Locals("agent_token_fp", servermw.AgentTokenFingerprint(plain))
	_ = h.db.Connectors().UpdateAgentHeartbeat(tok.ID, c.Get("X-Agent-Hostname"), c.Get("X-Agent-OS"), c.Get("X-Agent-Version"))
	return c.Next()
}

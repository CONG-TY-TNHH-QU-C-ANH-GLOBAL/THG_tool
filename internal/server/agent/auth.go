package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	servermw "github.com/thg/scraper/internal/server/middleware"
	"github.com/thg/scraper/internal/store/connectors"
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
		log.Printf("[AgentAuth] rejected token path=%s ip=%s fp=%s err=%v", c.Path(), c.IP(), agentTokenFingerprint(plain), err)
		return c.Status(401).JSON(fiber.Map{"error": "invalid or revoked agent token"})
	}
	c.Locals("agent_id", tok.ID)
	c.Locals("agent_org_id", tok.OrgID)
	c.Locals("agent_name", tok.Name)
	c.Locals("agent_created_by", tok.CreatedBy)
	c.Locals("agent_assigned_account_id", tok.AssignedAccountID)
	c.Locals("agent_token_fp", agentTokenFingerprint(plain))
	_ = h.db.Connectors().UpdateAgentHeartbeat(tok.ID, c.Get("X-Agent-Hostname"), c.Get("X-Agent-OS"), c.Get("X-Agent-Version"))
	return c.Next()
}

func agentTokenFingerprint(plain string) string {
	if strings.TrimSpace(plain) == "" {
		return "empty"
	}
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])[:12]
}

// clampPresenceFields enforces upper bounds on every connector-supplied field
// so a misbehaving runtime cannot bloat the agent_tokens or browser_sessions
// rows with unbounded strings. Called before UpdateAgentPresence /
// UpsertConnectorScreenshot.
func clampPresenceFields(p *connectors.AgentPresence) {
	servermw.ClampPresenceFields(p)
}

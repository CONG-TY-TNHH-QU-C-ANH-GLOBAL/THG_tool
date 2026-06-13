package agent

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/store"
)

// teardownConnectorBinding releases everything one connector binding holds:
// local browser sessions, streamed screenshots, and the token itself
// (active=0). Scope is exactly ONE connector — it never touches the THG user,
// workspace membership, account records, or other Chrome profiles' connectors.
// Shared by dashboard disconnect and the extension's self-disconnect.
func teardownConnectorBinding(db *store.Store, connectorID, orgID int64) error {
	conns, err := db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return err
	}
	if len(conns) <= 1 {
		_ = db.Connectors().StopAllLocalSessionsForOrg(orgID)
	} else {
		_ = db.Connectors().StopLocalSessionsForConnector(connectorID, orgID)
	}
	_ = db.Connectors().DeleteConnectorScreenshotsByAgent(connectorID, orgID)
	return db.Connectors().RevokeAgentToken(connectorID, orgID)
}

// agentSelfDisconnect lets the extension release its OWN binding when the
// operator clicks Forget Device — without it, the server row stays active and
// the typed "paired to another user" error tells the next user to do something
// that cannot unblock them. Token auth IS the identity: the token can only
// revoke itself, so no RBAC beyond agentAuth is needed.
// POST /api/connectors/self/disconnect
func (h *Handler) agentSelfDisconnect(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	kind, _ := c.Locals("agent_kind").(string)
	createdBy, _ := c.Locals("agent_created_by").(int64)
	if agentID <= 0 || orgID <= 0 {
		return c.Status(401).JSON(fiber.Map{"error": "invalid agent context"})
	}
	// Only extension connectors hold a binding to release. A worker-kind token
	// must not reach teardown — it is absent from the connector list, so the
	// len<=1 branch would stop ANOTHER member's live sessions.
	if kind != browsergateway.KindExtensionConnector {
		return c.Status(403).JSON(fiber.Map{"error": "not an extension connector"})
	}
	if err := teardownConnectorBinding(h.db, agentID, orgID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not release binding"})
	}
	h.db.InsertAuditLog(createdBy, "local_connector_self_disconnected", c.IP(), fmt.Sprintf(`{"connector_id":%d}`, agentID))
	return c.JSON(fiber.Map{"status": "revoked"})
}

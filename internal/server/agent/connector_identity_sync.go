package agent

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	serverorg "github.com/thg/scraper/internal/server/org"
)

// connectorIdentityConflictCode is the typed reason surfaced when a connector
// reports a Facebook identity already owned by ANOTHER member of the workspace.
// It matches the frontend pairing-status enum so the wizard can show the exact
// Vietnamese copy instead of a generic failure.
const connectorIdentityConflictCode = "facebook_account_already_connected_to_another_member"

// syncConnectorFacebookIdentity binds the logged-in Facebook identity carried by a
// connector presence snapshot to its OWNING workspace account — create-on-first-
// sight (owned by the pairing member), rebind the connector, persist the identity,
// mark the account active. It is the single source of truth shared by BOTH the
// heartbeat and chrome-status paths, so a connector materialises its Facebook
// account on the dashboard regardless of which presence call arrives first.
//
// Returns conflict=true when the FB account is owned by a different member (no
// mutation performed — caller should answer 409). err is a hard persistence
// failure. A snapshot with no org / no logged-in identity is a no-op.
func (h *Handler) syncConnectorFacebookIdentity(c *fiber.Ctx, agentID int64, snap serverorg.ConnectorIdentitySnapshot) (conflict bool, err error) {
	if snap.OrgID <= 0 || (snap.AccountID <= 0 && strings.TrimSpace(snap.FBUserID) == "") {
		return false, nil
	}
	createdBy, _ := c.Locals("agent_created_by").(int64)
	currentAssigned, _ := c.Locals("agent_assigned_account_id").(int64)
	res, err := serverorg.ResolveConnectorIdentity(h.db, c.Context(), agentID, createdBy, currentAssigned, snap)
	if err != nil {
		return false, err
	}
	// auditConnectorIdentity emits the create/rebind/conflict audit logs and
	// reports whether the caller must stop (ownership conflict).
	return h.auditConnectorIdentity(c, createdBy, snap.FBUserID, res), nil
}

// connectorIdentityConflictResponse writes the typed 409 for an ownership conflict.
// Shared so the heartbeat and chrome-status paths answer identically.
func connectorIdentityConflictResponse(c *fiber.Ctx) error {
	return c.Status(409).JSON(fiber.Map{
		"error":      "this Facebook account belongs to another member",
		"reason":     "ownership_conflict",
		"error_code": connectorIdentityConflictCode,
	})
}

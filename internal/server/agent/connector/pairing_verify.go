package connector

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store/connectors"
)

// pairingClaimErrorResponse maps typed pairing-claim errors to HTTP responses
// with a stable error_code, so the extension popup can show a specific
// Vietnamese message. Ownership-boundary violations are 409 (the code may be
// perfectly valid — the Chrome profile is the problem); code-lifecycle errors
// stay 400 like the legacy generic path.
func pairingClaimErrorResponse(c *fiber.Ctx, err error) error {
	code := connectors.PairingErrorCode(err)
	if code == "" {
		// Untyped = internal failure. This endpoint is unauthenticated —
		// never echo internal error text to it.
		return c.Status(500).JSON(fiber.Map{"error": "pairing failed, try again"})
	}
	status := 400
	switch {
	case errors.Is(err, connectors.ErrDevicePairedToAnotherUser),
		errors.Is(err, connectors.ErrDevicePairedToAnotherWorkspace):
		status = 409
	case errors.Is(err, connectors.ErrBrowserProfileRequired):
		// The code is fine; the extension is too old / not sending a stable
		// profile id. 426 Upgrade Required tells the client to update.
		status = 426
	}
	return c.Status(status).JSON(fiber.Map{"error": err.Error(), "error_code": code})
}

// getPairingFacebookStatus verifies the Facebook login of ONE pairing session.
// GET /api/connectors/pairing/:id/facebook-status
//
// Verification reads ONLY the connector token bound to this pairing session
// (connector_pairing_codes.device_token_id) — never the latest workspace
// heartbeat, never another user's connector, never another Chrome profile on
// the same laptop. Only the member who created the pairing session may verify
// it; that is the owner binding, so even an admin cannot verify through a
// staff member's device.
// buildPairingStatusResponse renders the pairing-status JSON. Pure; the wire shape
// (keys, types, and the detected-only fb fields) is identical to the inline block it
// replaced — extracted only to keep getPairingFacebookStatus under the complexity gate.
func buildPairingStatusResponse(session *connectors.ConnectorPairingSession, token *connectors.AgentToken, status connectors.PairingFacebookStatus) fiber.Map {
	resp := fiber.Map{"status": status, "pairing_session_id": session.ID}
	if token != nil {
		resp["connector_id"] = token.ID
		if status == connectors.PairingStatusDetected {
			resp["fb_user_id"] = token.FBUserID
			resp["fb_display_name"] = token.FBDisplayName
		}
		if token.LastSeen != nil {
			resp["last_proof_at"] = token.LastSeen.UTC().Format(time.RFC3339)
		}
	}
	return resp
}

func (h *LocalConnectorHandler) getPairingFacebookStatus(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid pairing session id"})
	}
	session, err := h.db.Connectors().GetConnectorPairingSession(int64(id), orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if session == nil {
		return c.Status(404).JSON(fiber.Map{"error": "pairing session not found"})
	}
	if session.CreatedBy != userID {
		return c.Status(403).JSON(fiber.Map{"error": "you can only verify your own pairing session"})
	}

	// Exact lookup, fail CLOSED: a DB error must not masquerade as
	// waiting_pairing and send the operator back to re-paste a consumed code.
	var token *connectors.AgentToken
	if session.DeviceTokenID > 0 {
		token, err = h.db.Connectors().GetLocalConnectorByID(session.DeviceTokenID, orgID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "verification unavailable, try again"})
		}
	}
	accountOwnerID := int64(0)
	if token != nil && token.FBUserID != "" {
		// Fail CLOSED: the owner lookup feeds the conflict gate; a swallowed
		// error here could report "detected" for another member's account.
		acc, err := h.db.Identities().GetAccountByFacebookIdentity(orgID, token.FBUserID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "verification unavailable, try again"})
		}
		if acc != nil {
			accountOwnerID = acc.AssignedUserID
		}
	}
	status := connectors.ResolvePairingFacebookStatus(session, token, accountOwnerID, time.Now().UTC())
	return c.JSON(buildPairingStatusResponse(session, token, status))
}

package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// listLocalConnectors returns desktop/extension connector devices for the org.
// GET /api/connectors
func (s *Server) listLocalConnectors(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	connectors, err := s.db.ListLocalConnectors(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	online := 0
	for _, conn := range connectors {
		if conn.Online {
			online++
		}
	}
	return c.JSON(fiber.Map{
		"connectors": connectors,
		"count":      len(connectors),
		"online":     online,
	})
}

// createLocalConnectorPairingCode creates a short-lived code for first-time desktop pairing.
// POST /api/connectors/pairing-code
func (s *Server) createLocalConnectorPairingCode(c *fiber.Ctx) error {
	var req struct {
		Name      string `json:"name"`
		AccountID int64  `json:"account_id"`
	}
	_ = c.BodyParser(&req)

	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	if orgID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("Local Chrome #%d", userID)
	}
	if req.AccountID > 0 {
		acc, err := s.db.GetAccount(req.AccountID)
		if err != nil || acc == nil || acc.OrgID != orgID {
			return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
		}
	}

	pair, err := s.db.CreateConnectorPairingCode(name, userID, orgID, req.AccountID, 10*time.Minute)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	s.db.InsertAuditLog(userID, "local_connector_pairing_created", c.IP(), fmt.Sprintf(`{"pairing_id":%d,"account_id":%d}`, pair.ID, req.AccountID))
	return c.Status(201).JSON(fiber.Map{
		"id":          pair.ID,
		"code":        pair.Code,
		"expires_at":  pair.ExpiresAt,
		"ttl_seconds": 600,
	})
}

// claimLocalConnectorPairingCode is called by THG Local Connector without JWT.
// POST /api/connectors/pair
func (s *Server) claimLocalConnectorPairingCode(c *fiber.Ctx) error {
	var req struct {
		Code             string `json:"code"`
		Hostname         string `json:"hostname"`
		OS               string `json:"os"`
		Version          string `json:"version"`
		CapabilitiesJSON string `json:"capabilities_json"`
		CurrentURL       string `json:"current_url"`
		FBUserID         string `json:"fb_user_id"`
		StreamStatus     string `json:"stream_status"`
	}
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "pairing code is required"})
	}
	tok, deviceToken, err := s.db.ClaimConnectorPairingCode(req.Code, store.AgentPresence{
		Hostname:         req.Hostname,
		OS:               req.OS,
		Version:          req.Version,
		Kind:             "desktop_connector",
		Transport:        "websocket",
		CapabilitiesJSON: req.CapabilitiesJSON,
		CurrentURL:       req.CurrentURL,
		FBUserID:         req.FBUserID,
		StreamStatus:     req.StreamStatus,
	})
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{
		"device_token": deviceToken,
		"connector":    tok,
		"ws_path":      "/ws/agent",
		"api_base":     "/api",
	})
}

// assignLocalConnectorAccount binds a connector to one Facebook account slot.
// PUT /api/connectors/:id/account
func (s *Server) assignLocalConnectorAccount(c *fiber.Ctx) error {
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	orgID, _ := c.Locals("org_id").(int64)
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid connector id"})
	}
	if req.AccountID > 0 {
		acc, err := s.db.GetAccount(req.AccountID)
		if err != nil || acc == nil || acc.OrgID != orgID {
			return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
		}
	}
	if err := s.db.AssignAgentAccount(int64(id), orgID, req.AccountID); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "connector not found"})
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

// revokeLocalConnector deactivates a connector token.
// DELETE /api/connectors/:id
func (s *Server) revokeLocalConnector(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid connector id"})
	}
	if err := s.db.RevokeAgentToken(int64(id), orgID); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "connector not found"})
	}
	return c.JSON(fiber.Map{"status": "revoked"})
}

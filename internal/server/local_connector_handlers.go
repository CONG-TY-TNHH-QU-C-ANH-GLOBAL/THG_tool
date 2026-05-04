package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
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

// getLocalConnectorScreen returns the latest screenshot streamed by a local connector.
// GET /api/connectors/screen?account_id=123
func (s *Server) getLocalConnectorScreen(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	accountID := int64(c.QueryInt("account_id", 0))
	screen, err := s.db.GetLatestConnectorScreenshot(orgID, accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	actions, err := s.db.RecentConnectorCommands(orgID, accountID, 8)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"screen": screen, "actions": actions})
}

// createConnectorInputCommand queues a dashboard mouse/keyboard command for THG Local Runtime.
// POST /api/connectors/input
func (s *Server) createConnectorInputCommand(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	var req struct {
		AccountID int64           `json:"account_id"`
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	switch req.Type {
	case "click", "key", "text", "scroll":
	default:
		return c.Status(400).JSON(fiber.Map{"error": "unsupported input command"})
	}
	if req.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "account_id is required"})
	}
	if acc, err := s.db.GetAccountForOrg(req.AccountID, orgID); err != nil || acc == nil {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}
	if len(req.Payload) == 0 || string(req.Payload) == "null" {
		req.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(req.Payload) {
		return c.Status(400).JSON(fiber.Map{"error": "payload must be valid JSON"})
	}
	if len(req.Payload) > 16*1024 {
		return c.Status(413).JSON(fiber.Map{"error": "input payload is too large"})
	}

	screen, err := s.db.GetLatestConnectorScreenshot(orgID, req.AccountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if screen == nil || screen.AgentID <= 0 {
		return c.Status(409).JSON(fiber.Map{"error": "local browser stream is not ready for this Facebook account"})
	}
	connectors, err := s.db.ListLocalConnectors(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	inputRelayReady := false
	for _, connector := range connectors {
		if connector.ID == screen.AgentID && connector.Active && localConnectorSupportsInputRelay(connector.CapabilitiesJSON) {
			inputRelayReady = true
			break
		}
	}
	if !inputRelayReady {
		return c.Status(409).JSON(fiber.Map{
			"error": "THG Local Runtime on this device is streaming video but does not support remote input yet",
			"hint":  "download the latest THG Local Kit, close the old runtime window, then start Start-THG-Local-Runtime again",
		})
	}
	id, err := s.db.CreateConnectorCommand(orgID, req.AccountID, screen.AgentID, userID, req.Type, string(req.Payload))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(202).JSON(fiber.Map{"status": "queued", "id": id})
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
		if acc, err := s.db.GetAccountForOrg(req.AccountID, orgID); err != nil || acc == nil {
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
		Kind             string `json:"kind"`
		Transport        string `json:"transport"`
		CapabilitiesJSON string `json:"capabilities_json"`
		CurrentURL       string `json:"current_url"`
		FBUserID         string `json:"fb_user_id"`
		StreamStatus     string `json:"stream_status"`
	}
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "pairing code is required"})
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "desktop_connector"
	}
	if kind != "desktop_connector" && kind != "extension_connector" {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported connector kind"})
	}
	transport := strings.TrimSpace(req.Transport)
	if transport == "" {
		if kind == "extension_connector" {
			transport = "chrome_extension"
		} else {
			transport = "local_chrome"
		}
	}
	tok, deviceToken, err := s.db.ClaimConnectorPairingCode(req.Code, store.AgentPresence{
		Hostname:         req.Hostname,
		OS:               req.OS,
		Version:          req.Version,
		Kind:             kind,
		Transport:        transport,
		CapabilitiesJSON: req.CapabilitiesJSON,
		CurrentURL:       req.CurrentURL,
		FBUserID:         req.FBUserID,
		StreamStatus:     req.StreamStatus,
	})
	if err != nil {
		log.Printf("[ConnectorPair] rejected code_fp=%s kind=%s transport=%s ip=%s err=%v", pairingCodeFingerprint(req.Code), kind, transport, c.IP(), err)
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[ConnectorPair] claimed connector_id=%d org_id=%d kind=%s transport=%s account_id=%d ip=%s token_fp=%s",
		tok.ID, tok.OrgID, tok.Kind, tok.Transport, tok.AssignedAccountID, c.IP(), agentTokenFingerprint(deviceToken))
	_ = s.db.InsertAuditLog(tok.CreatedBy, "local_connector_pairing_claimed", c.IP(),
		fmt.Sprintf(`{"connector_id":%d,"org_id":%d,"kind":%q,"transport":%q,"account_id":%d}`,
			tok.ID, tok.OrgID, tok.Kind, tok.Transport, tok.AssignedAccountID))
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
		if acc, err := s.db.GetAccountForOrg(req.AccountID, orgID); err != nil || acc == nil {
			return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
		}
	}
	if err := s.db.AssignAgentAccount(int64(id), orgID, req.AccountID); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "connector not found"})
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

// disconnectLocalConnector deactivates a connector token and clears local screen state.
func (s *Server) disconnectLocalConnector(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid connector id"})
	}
	connectors, err := s.db.ListLocalConnectors(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var found *store.AgentToken
	for i := range connectors {
		if connectors[i].ID == int64(id) {
			found = &connectors[i]
			break
		}
	}
	if found == nil {
		return c.Status(404).JSON(fiber.Map{"error": "connector not found"})
	}
	isAdmin := role == "admin" || models.IsPlatformRole(models.UserRole(role))
	if !isAdmin && found.CreatedBy != userID {
		return c.Status(403).JSON(fiber.Map{"error": "you can only disconnect your own device"})
	}
	_, _ = store.NewAppStore(s.db)
	if len(connectors) <= 1 {
		_ = s.db.StopAllLocalSessionsForOrg(orgID)
	} else {
		_ = s.db.StopLocalSessionsForConnector(int64(id), orgID)
	}
	_ = s.db.DeleteConnectorScreenshotsByAgent(int64(id), orgID)
	if err := s.db.RevokeAgentToken(int64(id), orgID); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "connector not found"})
	}
	s.db.InsertAuditLog(userID, "local_connector_disconnected", c.IP(), fmt.Sprintf(`{"connector_id":%d}`, id))
	return c.JSON(fiber.Map{"status": "revoked"})
}

// disconnectLocalConnectorPost is the dashboard button endpoint.
// POST /api/connectors/:id/disconnect
func (s *Server) disconnectLocalConnectorPost(c *fiber.Ctx) error {
	return s.disconnectLocalConnector(c)
}

// revokeLocalConnector keeps the DELETE API as a compatibility alias.
// DELETE /api/connectors/:id
func (s *Server) revokeLocalConnector(c *fiber.Ctx) error {
	return s.disconnectLocalConnector(c)
}

func pairingCodeFingerprint(code string) string {
	var normalized strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			normalized.WriteRune(r)
		}
	}
	if normalized.Len() == 0 {
		return "empty"
	}
	sum := sha256.Sum256([]byte(normalized.String()))
	return hex.EncodeToString(sum[:])[:12]
}

func localConnectorSupportsInputRelay(capabilitiesJSON string) bool {
	var caps map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(capabilitiesJSON)), &caps); err != nil {
		return false
	}
	return caps["input_relay"] == true
}

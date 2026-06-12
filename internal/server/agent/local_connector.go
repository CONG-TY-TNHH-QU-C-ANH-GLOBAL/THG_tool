package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

type LocalConnectorDeps struct {
	DB *store.Store
}

type LocalConnectorHandler struct {
	db *store.Store
}

func NewLocalConnectorHandler(deps LocalConnectorDeps) *LocalConnectorHandler {
	return &LocalConnectorHandler{db: deps.DB}
}

// LocalConnectorRoutes registers dashboard-authenticated connector routes.
func LocalConnectorRoutes(group fiber.Router, deps LocalConnectorDeps, adminOnly fiber.Handler) {
	h := NewLocalConnectorHandler(deps)
	group.Get("/connectors", h.listLocalConnectors)
	group.Get("/connectors/screen", h.getLocalConnectorScreen)
	group.Post("/connectors/input", h.createConnectorInputCommand)
	group.Post("/connectors", h.createLocalConnectorPairingCode)
	group.Post("/connectors/pairing-code", h.createLocalConnectorPairingCode)
	group.Get("/connectors/pairing/:id/facebook-status", h.getPairingFacebookStatus)
	group.Post("/connectors/:id/disconnect", h.disconnectLocalConnectorPost)
	group.Put("/connectors/:id/account", adminOnly, h.assignLocalConnectorAccount)
	group.Delete("/connectors/:id", h.revokeLocalConnector)
}

// LocalConnectorPairingRoutes registers unauthenticated first-pairing routes.
func LocalConnectorPairingRoutes(group fiber.Router, deps LocalConnectorDeps, pairingLimiter fiber.Handler) {
	h := NewLocalConnectorHandler(deps)
	group.Post("/connectors/pair", pairingLimiter, h.claimLocalConnectorPairingCode)
}

// listLocalConnectors returns Chrome Extension connector devices for the org.
// GET /api/connectors
func (h *LocalConnectorHandler) listLocalConnectors(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	all, err := h.db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// PR-M5 device privacy: a member sees ONLY the connectors (devices) they
	// paired — admin included. A staff member's Chrome/device never shows in
	// another member's list. (Server-side aggregates like the Nhân viên online
	// indicator read the full list separately and expose only a boolean.)
	conns := make([]connectors.AgentToken, 0, len(all))
	online := 0
	for _, conn := range all {
		if userID > 0 && conn.CreatedBy != userID {
			continue
		}
		conns = append(conns, conn)
		if conn.Online {
			online++
		}
	}
	return c.JSON(fiber.Map{
		"connectors": conns,
		"count":      len(conns),
		"online":     online,
	})
}

// getLocalConnectorScreen returns the latest screenshot streamed by a local connector.
// GET /api/connectors/screen?account_id=123
func (h *LocalConnectorHandler) getLocalConnectorScreen(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	accountID := int64(c.QueryInt("account_id", 0))
	screen, err := h.db.Connectors().GetLatestConnectorScreenshot(orgID, accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	actions, err := h.db.Connectors().RecentConnectorCommands(orgID, accountID, 8)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"screen": screen, "actions": actions})
}

// createConnectorInputCommand queues a dashboard mouse/keyboard command for THG Chrome Extension.
// POST /api/connectors/input
func (h *LocalConnectorHandler) createConnectorInputCommand(c *fiber.Ctx) error {
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
	acc, err := h.db.Identities().GetAccountForOrg(req.AccountID, orgID)
	if err != nil || acc == nil {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}
	// PR-2b: remote input = controlling the device streaming this account.
	// Device privacy (PR-M5) applies — only the owning member may drive it;
	// admin may drive unassigned org accounts only, NEVER a staff member's.
	// Previously only the org check above ran, letting any member queue
	// input against a colleague's browser.
	role, _ := c.Locals("user_role").(string)
	if !models.CanViewAccountDevice(acc, userID, role) {
		return c.Status(403).JSON(fiber.Map{"error": "you do not own this account's device"})
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

	screen, err := h.db.Connectors().GetLatestConnectorScreenshot(orgID, req.AccountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if screen == nil || screen.AgentID <= 0 {
		return c.Status(409).JSON(fiber.Map{"error": "local browser stream is not ready for this Facebook account"})
	}
	conns, err := h.db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	inputRelayReady := false
	for _, connector := range conns {
		if connector.ID == screen.AgentID && connector.Active && localConnectorSupportsInputRelay(connector.CapabilitiesJSON) {
			inputRelayReady = true
			break
		}
	}
	if !inputRelayReady {
		return c.Status(409).JSON(fiber.Map{
			"error": "THG Chrome Extension on this browser is streaming video but does not support remote input yet",
			"hint":  "reload the latest THG Chrome Extension package, then open the Facebook tab again",
		})
	}
	id, err := h.db.Connectors().CreateConnectorCommand(orgID, req.AccountID, screen.AgentID, userID, req.Type, string(req.Payload))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(202).JSON(fiber.Map{"status": "queued", "id": id})
}

// createLocalConnectorPairingCode creates a short-lived code for first-time Chrome Extension pairing.
// POST /api/connectors/pairing-code
func (h *LocalConnectorHandler) createLocalConnectorPairingCode(c *fiber.Ctx) error {
	var req struct {
		Name      string `json:"name"`
		AccountID int64  `json:"account_id"`
	}
	_ = c.BodyParser(&req)

	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	if orgID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("%s #%d", browsergateway.DefaultChromeConnectorName, userID)
	}
	// RBAC (Organic Sales Network): a member may only pre-bind a pairing code to
	// an account they OWN. Previously this only checked org membership, letting a
	// sales member create a code bound to another member's account. account_id==0
	// is allowed — that pairs the connector to the member, and the account is
	// auto-bound by Facebook identity on first login (PR2).
	if req.AccountID > 0 {
		acc, err := h.db.Identities().GetAccountForOrg(req.AccountID, orgID)
		if err != nil || acc == nil {
			return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
		}
		if !models.IsAccountOwnerAllowed(acc, userID, role) {
			return c.Status(403).JSON(fiber.Map{"error": "you do not own this account"})
		}
	}

	pair, err := h.db.Connectors().CreateConnectorPairingCode(name, userID, orgID, req.AccountID, 10*time.Minute)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	h.db.InsertAuditLog(userID, "local_connector_pairing_created", c.IP(), fmt.Sprintf(`{"pairing_id":%d,"account_id":%d}`, pair.ID, req.AccountID))
	return c.Status(201).JSON(fiber.Map{
		"id":          pair.ID,
		"code":        pair.Code,
		"expires_at":  pair.ExpiresAt,
		"ttl_seconds": 600,
	})
}

// claimLocalConnectorPairingCode is called by THG Chrome Extension without JWT.
// POST /api/connectors/pair
func (h *LocalConnectorHandler) claimLocalConnectorPairingCode(c *fiber.Ctx) error {
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
		FBDisplayName    string `json:"fb_display_name"`
		FBUsername       string `json:"fb_username"`
		FBProfileURL     string `json:"fb_profile_url"`
		StreamStatus     string `json:"stream_status"`

		IdentityConfidence       string `json:"identity_confidence"`
		IdentityExtractionMethod string `json:"identity_extraction_method"`
		IdentityLastVerifiedAt   string `json:"identity_last_verified_at"`
		BrowserProfileID         string `json:"browser_profile_id"`
		BuildNumber              string `json:"build_number"`
		ReleaseChannel           string `json:"release_channel"`
	}
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "pairing code is required"})
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = browsergateway.KindExtensionConnector
	}
	if kind != browsergateway.KindExtensionConnector {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported connector kind"})
	}
	transport := strings.TrimSpace(req.Transport)
	if transport == "" {
		transport = browsergateway.TransportChromeExtension
	}
	presence := connectors.AgentPresence{
		Hostname:                 req.Hostname,
		OS:                       req.OS,
		Version:                  req.Version,
		Kind:                     kind,
		Transport:                transport,
		CapabilitiesJSON:         req.CapabilitiesJSON,
		CurrentURL:               req.CurrentURL,
		FBUserID:                 req.FBUserID,
		FBDisplayName:            req.FBDisplayName,
		FBUsername:               req.FBUsername,
		FBProfileURL:             req.FBProfileURL,
		StreamStatus:             req.StreamStatus,
		IdentityConfidence:       req.IdentityConfidence,
		IdentityExtractionMethod: req.IdentityExtractionMethod,
		IdentityLastVerifiedAt:   req.IdentityLastVerifiedAt,
		BrowserProfileID:         req.BrowserProfileID,
		BuildNumber:              req.BuildNumber,
		ReleaseChannel:           req.ReleaseChannel,
	}
	// Same clamps as the heartbeat path — claim is unauthenticated and now
	// persists browser_profile_id into a UNIQUE-indexed column.
	clampPresenceFields(&presence)
	claimed, err := h.db.Connectors().ClaimConnectorPairingCode(req.Code, presence)
	if err != nil {
		log.Printf("[ConnectorPair] rejected code_fp=%s kind=%s transport=%s ip=%s err=%v", pairingCodeFingerprint(req.Code), kind, transport, c.IP(), err)
		return pairingClaimErrorResponse(c, err)
	}
	tok, deviceToken := claimed.Token, claimed.DeviceToken
	if strings.TrimSpace(req.BrowserProfileID) == "" {
		// Legacy extension (<0.5.55) — the profile-ownership guard was skipped.
		// Logged so silent downgrades of the boundary are observable.
		log.Printf("[ConnectorPair] legacy claim without browser_profile_id connector_id=%d org_id=%d version=%s ip=%s", tok.ID, tok.OrgID, req.Version, c.IP())
	}
	log.Printf("[ConnectorPair] claimed connector_id=%d org_id=%d kind=%s transport=%s account_id=%d ip=%s token_fp=%s",
		tok.ID, tok.OrgID, tok.Kind, tok.Transport, tok.AssignedAccountID, c.IP(), agentTokenFingerprint(deviceToken))
	_ = h.db.InsertAuditLog(tok.CreatedBy, "local_connector_pairing_claimed", c.IP(),
		fmt.Sprintf(`{"connector_id":%d,"org_id":%d,"kind":%q,"transport":%q,"account_id":%d}`,
			tok.ID, tok.OrgID, tok.Kind, tok.Transport, tok.AssignedAccountID))

	// Auto-bootstrap a local_starting browser_session row when the pair
	// already names a Facebook account. Without this the connector
	// heartbeats forever with "0 Chrome profile(s)" because
	// /api/agent/browser-targets requires a `local_*` session row to
	// surface a target. Forcing the operator to also click
	// "Start stream" on the dashboard right after pair is a UX dead
	// end — the intent of pair-with-account is "use this device for
	// that account, now". When pair has no account, we leave the
	// session row absent and the connector message tells the operator
	// to assign one from the dashboard.
	if tok.OrgID > 0 && tok.AssignedAccountID > 0 {
		if err := EnsureAssignedLocalBrowserTarget(h.db, c.Context(), tok.OrgID, tok.AssignedAccountID); err != nil {
			log.Printf("[ConnectorPair] auto-bootstrap session failed connector_id=%d account_id=%d: %v",
				tok.ID, tok.AssignedAccountID, err)
		}
	}

	return c.Status(201).JSON(fiber.Map{
		"device_token":       deviceToken,
		"connector":          tok,
		"pairing_session_id": claimed.PairingSessionID,
		"ws_path":            "/ws/agent",
		"api_base":           "/api",
	})
}

// assignLocalConnectorAccount binds a connector to one Facebook account slot.
// PUT /api/connectors/:id/account
func (h *LocalConnectorHandler) assignLocalConnectorAccount(c *fiber.Ctx) error {
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
		if acc, err := h.db.Identities().GetAccountForOrg(req.AccountID, orgID); err != nil || acc == nil {
			return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
		}
	}
	if err := h.db.Connectors().AssignAgentAccount(int64(id), orgID, req.AccountID); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "connector not found"})
	}
	if req.AccountID > 0 {
		if err := EnsureAssignedLocalBrowserTarget(h.db, c.Context(), orgID, req.AccountID); err != nil {
			log.Printf("[ConnectorAssign] auto-bootstrap session failed connector_id=%d account_id=%d: %v",
				id, req.AccountID, err)
		}
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

// disconnectLocalConnector deactivates a connector token and clears local screen state.
func (h *LocalConnectorHandler) disconnectLocalConnector(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid connector id"})
	}
	conns, err := h.db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var found *connectors.AgentToken
	for i := range conns {
		if conns[i].ID == int64(id) {
			found = &conns[i]
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
	if err := teardownConnectorBinding(h.db, int64(id), orgID); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "connector not found"})
	}
	h.db.InsertAuditLog(userID, "local_connector_disconnected", c.IP(), fmt.Sprintf(`{"connector_id":%d}`, id))
	return c.JSON(fiber.Map{"status": "revoked"})
}

// disconnectLocalConnectorPost is the dashboard button endpoint.
// POST /api/connectors/:id/disconnect
func (h *LocalConnectorHandler) disconnectLocalConnectorPost(c *fiber.Ctx) error {
	return h.disconnectLocalConnector(c)
}

// revokeLocalConnector keeps the DELETE API as a compatibility alias.
// DELETE /api/connectors/:id
func (h *LocalConnectorHandler) revokeLocalConnector(c *fiber.Ctx) error {
	return h.disconnectLocalConnector(c)
}

func EnsureAssignedLocalBrowserTarget(db *store.Store, ctx context.Context, orgID, accountID int64) error {
	if orgID <= 0 || accountID <= 0 {
		return nil
	}
	acc, err := db.Identities().GetAccountForOrg(accountID, orgID)
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("account %d does not belong to organization %d", accountID, orgID)
	}
	if acc.Platform != models.PlatformFacebook {
		return fmt.Errorf("account %d is not a Facebook account", accountID)
	}
	appStore, err := store.NewAppStore(db)
	if err != nil {
		return err
	}
	if sess, err := appStore.GetSession(ctx, accountID); err == nil && sess != nil {
		status := strings.ToLower(strings.TrimSpace(sess.Status))
		if strings.HasPrefix(status, "local_") && status != "local_stopped" && status != "local_terminated" && status != "terminated" {
			return nil
		}
	}
	return appStore.RecordLocalSession(ctx, accountID, orgID, store.SessionStarting, "")
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
	return caps[browsergateway.CapabilityInputRelay] == true ||
		caps[browsergateway.CapabilityCommandPolling] == true ||
		caps["extension_bridge"] == "supported"
}

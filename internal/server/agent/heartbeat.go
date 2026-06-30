package agent

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/models"
	servermw "github.com/thg/scraper/internal/server/middleware"
	serverorg "github.com/thg/scraper/internal/server/org"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// auditConnectorIdentity emits the state-change audit logs for a resolved
// connector identity (create / rebind / conflict) — only on actual change, so a
// continuous heartbeat does not spam the audit trail. Returns true when the
// caller should stop (ownership conflict → 409).
func (h *Handler) auditConnectorIdentity(c *fiber.Ctx, createdBy int64, fbUserID string, res serverorg.ResolvedConnectorIdentity) bool {
	if res.Conflict {
		h.db.InsertAuditLog(createdBy, "account_ownership_conflict", c.IP(),
			fmt.Sprintf(`{"account_id":%d,"fb_user_id":%q,"owner_user_id":%d}`, res.AccountID, fbUserID, res.ConflictOwnerID))
		return true
	}
	if res.Created {
		h.db.InsertAuditLog(createdBy, "account_auto_created", c.IP(),
			fmt.Sprintf(`{"account_id":%d,"fb_user_id":%q}`, res.AccountID, fbUserID))
	}
	if res.Rebound {
		h.db.InsertAuditLog(createdBy, "connector_account_rebound", c.IP(),
			fmt.Sprintf(`{"from_account_id":%d,"to_account_id":%d}`, res.PreviousAccount, res.AccountID))
	}
	return false
}

// agentHeartbeat is a lightweight ping for connection health checks.
// POST /api/agent/heartbeat
func (h *Handler) agentHeartbeat(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	var body struct {
		Hostname                 string `json:"hostname"`
		OS                       string `json:"os"`
		Version                  string `json:"version"`
		Kind                     string `json:"kind"`
		Transport                string `json:"transport"`
		AccountID                int64  `json:"account_id"`
		CapabilitiesJSON         string `json:"capabilities_json"`
		CurrentURL               string `json:"current_url"`
		FBUserID                 string `json:"fb_user_id"`
		FBDisplayName            string `json:"fb_display_name"`
		FBUsername               string `json:"fb_username"`
		FBProfileURL             string `json:"fb_profile_url"`
		LoginEmail               string `json:"login_email"`
		StreamStatus             string `json:"stream_status"`
		ChromeError              string `json:"chrome_error"`
		IdentityConfidence       string `json:"identity_confidence"`
		IdentityExtractionMethod string `json:"identity_extraction_method"`
		IdentityLastVerifiedAt   string `json:"identity_last_verified_at"`
		BrowserProfileID         string `json:"browser_profile_id"`
		BuildNumber              string `json:"build_number"`
		ReleaseChannel           string `json:"release_channel"`
	}
	_ = c.BodyParser(&body)
	if body.Hostname == "" {
		body.Hostname = c.Get("X-Agent-Hostname")
	}
	if body.OS == "" {
		body.OS = c.Get("X-Agent-OS")
	}
	if body.Version == "" {
		body.Version = c.Get("X-Agent-Version")
	}
	presence := connectors.AgentPresence{
		Hostname:                 body.Hostname,
		OS:                       body.OS,
		Version:                  body.Version,
		Kind:                     body.Kind,
		Transport:                body.Transport,
		AssignedAccountID:        body.AccountID,
		CapabilitiesJSON:         body.CapabilitiesJSON,
		CurrentURL:               body.CurrentURL,
		FBUserID:                 body.FBUserID,
		FBDisplayName:            body.FBDisplayName,
		FBUsername:               body.FBUsername,
		FBProfileURL:             body.FBProfileURL,
		StreamStatus:             body.StreamStatus,
		ChromeError:              body.ChromeError,
		IdentityConfidence:       body.IdentityConfidence,
		IdentityExtractionMethod: body.IdentityExtractionMethod,
		IdentityLastVerifiedAt:   body.IdentityLastVerifiedAt,
		BrowserProfileID:         body.BrowserProfileID,
		BuildNumber:              body.BuildNumber,
		ReleaseChannel:           body.ReleaseChannel,
	}
	servermw.ClampPresenceFields(&presence)
	_ = h.db.Connectors().UpdateAgentPresence(agentID, presence)
	// PR-8: blocked extension builds raise a rate-limited alert
	// (staff + admin in-app, optional Telegram). Never per-heartbeat.
	h.maybeAlertExtensionOutdated(orgID, agentID, presence.Version)
	// Organic Sales Network: the heartbeat carries the logged-in Facebook identity.
	// Bind it to its owning workspace account (create-on-first-sight, no-steal) so a
	// connector paired WITHOUT a pre-created account still materialises the FB account
	// on the dashboard. Previously ONLY chrome-status bound it, and chrome-status was
	// gated behind an existing browser target — which requires an account to already
	// exist — deadlocking every first-time connection (popup showed a session, the
	// dashboard stayed at 0 accounts forever).
	conflict, bindErr := h.syncConnectorFacebookIdentity(c, agentID, serverorg.ConnectorIdentitySnapshot{
		AccountID: body.AccountID, OrgID: orgID, StreamStatus: body.StreamStatus,
		CurrentURL: body.CurrentURL, FBUserID: body.FBUserID, FBDisplayName: body.FBDisplayName,
		FBUsername: body.FBUsername, FBProfileURL: body.FBProfileURL, LoginEmail: body.LoginEmail,
		ChromeError: body.ChromeError,
	})
	if conflict {
		return connectorIdentityConflictResponse(c)
	}
	if bindErr != nil {
		// Liveness must not depend on identity binding (heartbeat/work decoupling).
		// Log so a persistence failure is visible server-side rather than silently
		// surfacing as an empty dashboard; last_seen was already refreshed above.
		log.Printf("[Heartbeat] FB identity bind failed agent=%d org=%d fb_user=%q: %v", agentID, orgID, body.FBUserID, bindErr)
	}
	return c.JSON(fiber.Map{
		"status":       "ok",
		"connector_id": agentID,
		"org_id":       orgID,
		"ts":           time.Now().Unix(),
	})
}

// agentChromeStatus is the explicit Chrome Extension handshake endpoint.
// The extension calls this even before any workspace target exists, so
// the dashboard can distinguish "device paired" from "Facebook tab ready".
// POST /api/agent/chrome-status
func (h *Handler) agentChromeStatus(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	var body struct {
		AccountID                int64  `json:"account_id"`
		CurrentURL               string `json:"current_url"`
		FBUserID                 string `json:"fb_user_id"`
		FBDisplayName            string `json:"fb_display_name"`
		FBUsername               string `json:"fb_username"`
		FBProfileURL             string `json:"fb_profile_url"`
		LoginEmail               string `json:"login_email"`
		StreamStatus             string `json:"stream_status"`
		ChromeError              string `json:"chrome_error"`
		IdentityConfidence       string `json:"identity_confidence"`
		IdentityExtractionMethod string `json:"identity_extraction_method"`
		IdentityLastVerifiedAt   string `json:"identity_last_verified_at"`
		BrowserProfileID         string `json:"browser_profile_id"`
		BuildNumber              string `json:"build_number"`
		ReleaseChannel           string `json:"release_channel"`
	}
	_ = c.BodyParser(&body)

	status := strings.TrimSpace(body.StreamStatus)
	if status == "" {
		status = browsergateway.StreamChromeNotConnected
	}
	presence := connectors.AgentPresence{
		AssignedAccountID:        body.AccountID,
		CurrentURL:               body.CurrentURL,
		FBUserID:                 body.FBUserID,
		FBDisplayName:            body.FBDisplayName,
		FBUsername:               body.FBUsername,
		FBProfileURL:             body.FBProfileURL,
		StreamStatus:             status,
		ChromeError:              body.ChromeError,
		IdentityConfidence:       body.IdentityConfidence,
		IdentityExtractionMethod: body.IdentityExtractionMethod,
		IdentityLastVerifiedAt:   body.IdentityLastVerifiedAt,
		BrowserProfileID:         body.BrowserProfileID,
		BuildNumber:              body.BuildNumber,
		ReleaseChannel:           body.ReleaseChannel,
	}
	servermw.ClampPresenceFields(&presence)
	_ = h.db.Connectors().UpdateAgentPresence(agentID, presence)
	// PR-8: blocked extension builds raise a rate-limited alert
	// (staff + admin in-app, optional Telegram). Never per-heartbeat.
	h.maybeAlertExtensionOutdated(orgID, agentID, presence.Version)
	// Resolve the logged-in Facebook identity to its owning account (auto-create
	// owned by the pairing member, rebind the connector, no-steal) — shared with the
	// heartbeat path via syncConnectorFacebookIdentity.
	conflict, err := h.syncConnectorFacebookIdentity(c, agentID, serverorg.ConnectorIdentitySnapshot{
		AccountID: body.AccountID, OrgID: orgID, StreamStatus: status,
		CurrentURL: body.CurrentURL, FBUserID: body.FBUserID, FBDisplayName: body.FBDisplayName,
		FBUsername: body.FBUsername, FBProfileURL: body.FBProfileURL, LoginEmail: body.LoginEmail,
		ChromeError: body.ChromeError,
	})
	if err != nil {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	if conflict {
		return connectorIdentityConflictResponse(c)
	}
	return c.JSON(fiber.Map{
		"status":        "ok",
		"stream_status": status,
		"ts":            time.Now().Unix(),
	})
}

// agentBrowserTargets returns the org account slots that should run on local Chrome.
// GET /api/agent/browser-targets
//
// When the result is empty we attach a `hint` so the connector console can
// tell the operator exactly what is missing; without this the connector
// just prints "0 Chrome profile(s)" forever and the operator does not know
// whether the dashboard is broken or whether they forgot a step.
func (h *Handler) agentBrowserTargets(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	agentID, _ := c.Locals("agent_id").(int64)
	createdBy, _ := c.Locals("agent_created_by").(int64)
	assignedAccountID, _ := c.Locals("agent_assigned_account_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	if _, err := store.NewAppStore(h.db); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	targets, err := h.db.Connectors().ListLocalBrowserTargetsForConnector(orgID, agentID, createdBy, assignedAccountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if len(targets) == 0 && assignedAccountID > 0 {
		if err := EnsureAssignedLocalBrowserTarget(h.db, c.Context(), orgID, assignedAccountID); err != nil {
			log.Printf("[AgentTargets] auto-bootstrap failed org_id=%d agent_id=%d account_id=%d: %v",
				orgID, agentID, assignedAccountID, err)
		} else if refreshed, err := h.db.Connectors().ListLocalBrowserTargetsForConnector(orgID, agentID, createdBy, assignedAccountID); err == nil {
			targets = refreshed
		}
	}
	resp := fiber.Map{
		"targets":             targets,
		"count":               len(targets),
		"assigned_account_id": assignedAccountID,
	}
	if len(targets) == 0 {
		resp["hint_code"], resp["hint"] = browserTargetsHint(h, orgID, assignedAccountID)
	}
	return c.JSON(resp)
}

// browserTargetsHint inspects the org's account state to explain why the
// connector has no runnable browser target yet. The short code is stable for
// Chrome Extension clients; the prose is a fallback for older clients.
func browserTargetsHint(h *Handler, orgID, assignedAccountID int64) (string, string) {
	if orgID <= 0 {
		return "no_org", "Chrome Extension chưa được gắn vào workspace nào. Pair lại bằng mã mới từ Browser dashboard."
	}
	accounts, _ := h.db.Identities().GetAllAccounts(orgID)
	hasFacebook := false
	assignedExists := assignedAccountID <= 0
	for _, a := range accounts {
		if a.Platform == models.PlatformFacebook {
			hasFacebook = true
			if assignedAccountID > 0 && a.ID == assignedAccountID {
				assignedExists = true
			}
		}
	}
	if !hasFacebook {
		return "no_account_in_org",
			"Workspace chưa có Facebook account. Tạo phiên Facebook mới trong Browser dashboard; Chrome Extension sẽ tự nhận target khi account được tạo."
	}
	if assignedAccountID > 0 {
		if !assignedExists {
			return "assigned_account_missing",
				"Chrome Extension đang gắn với một Facebook account không còn tồn tại trong workspace. Disconnect thiết bị và tạo mã kết nối mới."
		}
		return "assigned_account_not_started",
			"Facebook account đã gắn với Chrome Extension nhưng chưa có Browser stream. Mở tab Facebook đã đăng nhập trong Chrome đó để extension stream về dashboard."
	}
	return "no_local_session_yet",
		"Chrome Extension đã online nhưng chưa được gắn với Facebook account cụ thể. Vào Browser dashboard, chọn account và tạo mã kết nối riêng cho Chrome này."
}

// agentScreenshot stores the latest observable frame from the user's real Chrome.
// POST /api/agent/screenshot
func (h *Handler) agentScreenshot(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}

	var body struct {
		AccountID     int64  `json:"account_id"`
		ImageData     string `json:"image_data"`
		CurrentURL    string `json:"current_url"`
		FBUserID      string `json:"fb_user_id"`
		FBDisplayName string `json:"fb_display_name"`
		FBUsername    string `json:"fb_username"`
		FBProfileURL  string `json:"fb_profile_url"`
		LoginEmail    string `json:"login_email"`
		StreamStatus  string `json:"stream_status"`
		ChromeError   string `json:"chrome_error"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	body.ImageData = strings.TrimSpace(body.ImageData)
	if body.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "account_id is required"})
	}
	if !strings.HasPrefix(body.ImageData, "data:image/jpeg;base64,") && !strings.HasPrefix(body.ImageData, "data:image/png;base64,") {
		return c.Status(400).JSON(fiber.Map{"error": "image_data must be a data URL"})
	}
	if len(body.ImageData) > 6*1024*1024 {
		return c.Status(413).JSON(fiber.Map{"error": "screenshot is too large"})
	}

	createdBy, _ := c.Locals("agent_created_by").(int64)
	currentAssigned, _ := c.Locals("agent_assigned_account_id").(int64)
	streamStatus := strings.TrimSpace(body.StreamStatus)
	if streamStatus == "" {
		streamStatus = browsergateway.StreamConnectorOnline
	}
	// Resolve the Facebook identity to its owning account (auto-create + rebind,
	// no-steal) BEFORE storing the screenshot, so the screenshot attaches to the
	// account that actually owns this FB login — not a stale extension slot.
	res, err := serverorg.ResolveConnectorIdentity(h.db, c.Context(), agentID, createdBy, currentAssigned, serverorg.ConnectorIdentitySnapshot{
		AccountID:     body.AccountID,
		OrgID:         orgID,
		StreamStatus:  streamStatus,
		CurrentURL:    body.CurrentURL,
		FBUserID:      body.FBUserID,
		FBDisplayName: body.FBDisplayName,
		FBUsername:    body.FBUsername,
		FBProfileURL:  body.FBProfileURL,
		LoginEmail:    body.LoginEmail,
		ChromeError:   body.ChromeError,
	})
	if err != nil {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	if h.auditConnectorIdentity(c, createdBy, body.FBUserID, res) {
		return c.Status(409).JSON(fiber.Map{"error": "this Facebook account belongs to another member", "reason": "ownership_conflict"})
	}
	accountID := res.AccountID
	if accountID <= 0 {
		accountID = body.AccountID
	}
	if err := h.db.Connectors().UpsertConnectorScreenshot(agentID, orgID, accountID, body.ImageData, body.CurrentURL, body.FBUserID, body.FBDisplayName, body.FBUsername, body.FBProfileURL, streamStatus, body.ChromeError); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	presence := connectors.AgentPresence{
		CurrentURL:    body.CurrentURL,
		FBUserID:      body.FBUserID,
		FBDisplayName: body.FBDisplayName,
		FBUsername:    body.FBUsername,
		FBProfileURL:  body.FBProfileURL,
		StreamStatus:  streamStatus,
		ChromeError:   body.ChromeError,
	}
	servermw.ClampPresenceFields(&presence)
	_ = h.db.Connectors().UpdateAgentPresence(agentID, presence)
	// PR-8: blocked extension builds raise a rate-limited alert
	// (staff + admin in-app, optional Telegram). Never per-heartbeat.
	h.maybeAlertExtensionOutdated(orgID, agentID, presence.Version)

	// Window Respect (PR-2): the periodic window_control:minimize was removed. It
	// existed for the old "observe automation via the dashboard BrowserView" model
	// (the raw tab was minimized out of the way) — but BrowserView/stream was
	// retired (PR-F), so the command only snapped the user's full-screen Chrome away
	// for a feature that no longer exists. The user-owned window is sacred; the
	// extension also no-ops window_control:minimize unless a debug policy opts in.

	return c.JSON(fiber.Map{"status": "stored", "ts": time.Now().Unix()})
}

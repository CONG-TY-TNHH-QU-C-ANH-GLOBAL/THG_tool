package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	serveragent "github.com/thg/scraper/internal/server/agent"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// workspaceList returns all Facebook accounts with their live browser status.
// GET /api/browser/workspaces
//
// RBAC-1: sales staff see only accounts assigned to them. Admin / platform
// see every account in the org. Execution-layer scoping per
// feedback_shared_battlefield_not_crm.md — leads stay shared elsewhere.
func (h *Handler) workspaceList(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	var (
		accounts []models.Account
		err      error
	)
	r := models.UserRole(role)
	if models.IsPlatformRole(r) || r == models.RoleAdmin {
		accounts, err = h.db.Identities().GetAllAccounts(orgID)
	} else {
		accounts, err = h.db.Identities().GetAccountsForUser(orgID, userID)
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	type entry struct {
		AccountID     int64      `json:"account_id"`
		AccountName   string     `json:"account_name"`
		Email         string     `json:"email,omitempty"`
		Status        string     `json:"account_status"`
		LoggedIn      bool       `json:"logged_in"`
		FBUserID      string     `json:"fb_user_id,omitempty"`
		FBDisplayName string     `json:"fb_display_name,omitempty"`
		FBUsername    string     `json:"fb_username,omitempty"`
		FBProfileURL  string     `json:"fb_profile_url,omitempty"`
		Running       bool       `json:"running"`
		CDPPort       int        `json:"cdp_port,omitempty"`
		VNCPort       int        `json:"vnc_port,omitempty"`
		StartedAt     *time.Time `json:"started_at,omitempty"`
		BrowserState  string     `json:"browser_state,omitempty"`
		ErrorMsg      string     `json:"error_msg,omitempty"`
	}

	appStore, _ := store.NewAppStore(h.db)
	_, localConnectorOnline := h.localConnectorAvailability(orgID)
	result := make([]entry, 0, len(accounts))
	for _, acc := range accounts {
		e := entry{
			AccountID:     acc.ID,
			AccountName:   acc.Name,
			Email:         acc.Email,
			Status:        string(acc.Status),
			LoggedIn:      acc.BrowserLoggedIn,
			FBUserID:      acc.FBUserID,
			FBDisplayName: acc.FBDisplayName,
			FBUsername:    acc.FBUsername,
			FBProfileURL:  acc.FBProfileURL,
		}
		if h.workspace != nil {
			if inst := h.workspaceInstanceForAccount(acc.ID, acc.Name); inst != nil {
				e.Running = true
				e.CDPPort = inst.CDPPort
				e.VNCPort = inst.VNCPort
				t := inst.StartedAt
				e.StartedAt = &t
			}
		}
		if appStore != nil {
			if sess, err := appStore.GetSession(c.Context(), acc.ID); err == nil && sess != nil && sess.Status != "terminated" {
				e.BrowserState = sess.Status
				e.ErrorMsg = sess.ErrorMsg
				if strings.HasPrefix(sess.Status, "local_") {
					e.Running = localConnectorOnline && sess.Status != "local_stopped" && sess.Status != "local_error"
				}
				if e.CDPPort == 0 {
					e.CDPPort = sess.CDPPort
				}
				if e.VNCPort == 0 {
					e.VNCPort = sess.VNCPort
				}
				if e.StartedAt == nil {
					t := sess.StartedAt
					e.StartedAt = &t
				}
			}
		}
		result = append(result, e)
	}
	return c.JSON(fiber.Map{"workspaces": result, "count": len(result)})
}

// workspaceStart launches a Docker browser for a specific account.
// Readiness is tracked asynchronously so production proxies never time out.
// POST /api/browser/workspaces/:id/start
func (h *Handler) workspaceStart(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	// RBAC-1: only the owner (or admin) may start this account's browser.
	acc, err := serveragent.RequireAccountOwner(h.db, c, id, orgID, userID, role)
	if err != nil {
		return err
	}
	_ = acc.Name // acc already validated; keep name access via acc.Name below
	hasOrgLocalConnector, _ := h.localConnectorAvailability(orgID)
	hasLocalConnector, hasOnlineLocalConnector := h.localConnectorAvailabilityForUser(orgID, userID, id)
	if hasOnlineLocalConnector {
		if err := h.recordLocalBrowserSession(id, orgID, store.SessionStarting, ""); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		_ = h.db.Identities().UpdateAccountStatus(id, models.AccountActive)
		return c.JSON(fiber.Map{
			"status":     "local_starting",
			"account_id": id,
			"local":      true,
		})
	}
	if hasLocalConnector {
		return c.Status(409).JSON(fiber.Map{
			"error": "THG Chrome Extension của bạn đang offline. Mở Chrome, bật extension đã ghép với workspace này, rồi bấm Bắt đầu stream lại.",
			"code":  "LOCAL_CONNECTOR_OFFLINE",
		})
	}
	if hasOrgLocalConnector {
		return c.Status(409).JSON(fiber.Map{
			"error": "Workspace đã có Chrome Extension online, nhưng chưa có extension nào gắn với user hoặc Facebook account này. Hãy tạo mã kết nối riêng trên dashboard này và ghép Chrome của bạn.",
			"code":  "LOCAL_CONNECTOR_NOT_ASSIGNED",
		})
	}
	if h.workspace == nil {
		return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
	}

	inst, err := h.workspace.Start(id, acc.Name)
	if err != nil {
		log.Printf("[Workspace] Failed to start container for account %d: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("container start failed: %v", err)})
	}

	_ = h.db.Identities().UpdateAccountStatus(id, models.AccountActive)
	h.recordBrowserSession(id, orgID, inst, "initializing", "")
	go h.watchWorkspaceReadiness(id, orgID, inst)
	// Login/checkpoint must remain a VNC-only human flow by default. CDP polling
	// during Meta verification can destabilize the browser session.
	if os.Getenv("WORKSPACE_AUTO_LOGIN_WATCHER") == "1" {
		go h.watchWorkspaceLogin(id, orgID, inst)
	}

	log.Printf("[Workspace] Account %d (%s) browser starting, vnc=%d cdp=%d", id, acc.Name, inst.VNCPort, inst.CDPPort)
	return c.JSON(fiber.Map{
		"status":     "starting",
		"account_id": id,
		"vnc_port":   inst.VNCPort,
		"cdp_port":   inst.CDPPort,
	})
}

// workspaceStop kills the Docker browser for a specific account.
// POST /api/browser/workspaces/:id/stop
//
// RBAC-1: only the owner (or admin) may stop this account's browser.
func (h *Handler) workspaceStop(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	if orgID != 0 {
		if _, err := serveragent.RequireAccountOwner(h.db, c, id, orgID, userID, role); err != nil {
			return err
		}
	}
	if h.workspace != nil {
		h.workspace.Stop(id)
	}
	if appStore, err := store.NewAppStore(h.db); err == nil {
		_ = appStore.TerminateSession(context.Background(), id)
	}
	return c.JSON(fiber.Map{"status": "stopped"})
}

func (h *Handler) localConnectorAvailability(orgID int64) (bool, bool) {
	return h.localConnectorAvailabilityForUser(orgID, 0, 0)
}

func (h *Handler) localConnectorAvailabilityForUser(orgID, userID, accountID int64) (bool, bool) {
	if orgID <= 0 {
		return false, false
	}
	connectors, err := h.db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		return false, false
	}
	hasAny := false
	hasOnline := false
	for _, conn := range connectors {
		if userID > 0 && conn.CreatedBy != userID && (accountID <= 0 || conn.AssignedAccountID != accountID) {
			continue
		}
		if conn.Active {
			hasAny = true
			if conn.Online && isDashboardStreamConnector(conn) {
				hasOnline = true
			}
		}
	}
	return hasAny, hasOnline
}

func isDashboardStreamConnector(conn connectors.AgentToken) bool {
	if conn.Kind == "extension_connector" || conn.Transport == "chrome_extension" {
		return true
	}
	var caps map[string]any
	if err := json.Unmarshal([]byte(conn.CapabilitiesJSON), &caps); err == nil {
		if v, ok := caps["chrome_extension"].(bool); ok && v {
			return true
		}
		if v, ok := caps["dashboard_stream"].(bool); ok && v {
			return true
		}
		if v, ok := caps["dom_metadata"].(bool); ok && v {
			return true
		}
		if v, ok := caps["extension_bridge"].(string); ok && v == "supported" {
			return true
		}
	}
	return false
}

// recordLocalBrowserSession is kept as a thin wrapper around AppStore.RecordLocalSession
// so legacy call sites continue to compile. New code should call AppStore directly.
func (h *Handler) recordLocalBrowserSession(accountID, orgID int64, status store.LocalSessionStatus, errorMsg string) error {
	appStore, err := store.NewAppStore(h.db)
	if err != nil {
		return err
	}
	return appStore.RecordLocalSession(context.Background(), accountID, orgID, status, errorMsg)
}

// workspaceNew creates a fresh Facebook account and starts its browser.
// It returns as soon as Docker launches; CDP/VNC readiness is tracked async.
// POST /api/browser/workspaces/new
func (h *Handler) workspaceNew(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	hasOrgLocalConnector, _ := h.localConnectorAvailability(orgID)
	hasLocalConnector, hasOnlineLocalConnector := h.localConnectorAvailabilityForUser(orgID, userID, 0)
	if hasLocalConnector && !hasOnlineLocalConnector {
		return c.Status(409).JSON(fiber.Map{
			"error": "THG Chrome Extension của bạn đang offline. Mở Chrome, bật extension đã ghép với workspace này, rồi tạo phiên Facebook lại.",
			"code":  "LOCAL_CONNECTOR_OFFLINE",
		})
	}
	if !hasLocalConnector && hasOrgLocalConnector {
		return c.Status(409).JSON(fiber.Map{
			"error": "Workspace đang dùng Chrome Extension, nhưng chưa có extension nào gắn với user của bạn. Mỗi nhân viên cần tự tạo mã kết nối và ghép Chrome riêng trước khi tạo phiên Facebook.",
			"code":  "LOCAL_CONNECTOR_NOT_ASSIGNED",
		})
	}

	name := fmt.Sprintf("Facebook %s", time.Now().Format("02/01 15:04"))
	acc := &models.Account{
		OrgID:          orgID,
		Platform:       models.PlatformFacebook,
		Name:           name,
		Status:         models.AccountInactive,
		AssignedUserID: userID,
	}
	id, err := h.db.Identities().AddAccount(acc)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "create account: " + err.Error()})
	}
	if hasOnlineLocalConnector {
		if err := h.recordLocalBrowserSession(id, orgID, store.SessionStarting, ""); err != nil {
			_ = h.db.Identities().DeleteAccount(id)
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		_ = h.db.Identities().UpdateAccountStatus(id, models.AccountActive)
		log.Printf("[Workspace] New local session requested: account %d (%s)", id, name)
		return c.JSON(fiber.Map{
			"status":     "local_starting",
			"account_id": id,
			"local":      true,
		})
	}
	if h.workspace == nil {
		_ = h.db.Identities().DeleteAccount(id)
		return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
	}

	inst, err := h.workspace.Start(id, name)
	if err != nil {
		_ = h.db.Identities().DeleteAccount(id)
		log.Printf("[Workspace] Failed to start new session for account %d: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "container start failed: " + err.Error()})
	}

	_ = h.db.Identities().UpdateAccountStatus(id, models.AccountActive)
	h.recordBrowserSession(id, orgID, inst, "initializing", "")
	go h.watchWorkspaceReadiness(id, orgID, inst)
	// Login/checkpoint must remain a VNC-only human flow by default. CDP polling
	// during Meta verification can destabilize the browser session.
	if os.Getenv("WORKSPACE_AUTO_LOGIN_WATCHER") == "1" {
		go h.watchWorkspaceLogin(id, orgID, inst)
	}

	log.Printf("[Workspace] New session starting: account %d (%s) vnc=%d cdp=%d", id, name, inst.VNCPort, inst.CDPPort)
	return c.JSON(fiber.Map{
		"status":     "starting",
		"account_id": id,
		"vnc_port":   inst.VNCPort,
		"cdp_port":   inst.CDPPort,
	})
}

// workspaceNavigate is a no-op in Docker/VNC mode.
// Navigation happens directly in the browser via mouse/keyboard through the viewer.
// POST /api/browser/workspaces/:id/navigate
func (h *Handler) workspaceNavigate(c *fiber.Ctx) error {
	return c.Status(501).JSON(fiber.Map{
		"error": "navigate is not available in browser-view mode; use the browser directly via the dashboard",
	})
}

// workspaceSyncSession probes the running browser and persists the Facebook
// identity when the c_user cookie is available.
// POST /api/browser/workspaces/:id/sync-session
func (h *Handler) workspaceSyncSession(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	orgID, _ := c.Locals("org_id").(int64)
	acc, err := h.db.Identities().GetAccountForOrg(id, orgID)
	if err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	if h.workspace == nil {
		return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
	}
	inst := h.workspaceInstanceForAccount(id, acc.Name)
	if inst == nil || inst.CDPPort == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "browser is not running"})
	}

	snap, err := facebookSessionSnapshotFromInstance(inst)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "session probe failed: " + err.Error()})
	}
	snap.AccountID = id
	snap.AccountName = acc.Name
	snap.StoredFBID = acc.FBUserID

	if snap.FBUserID != "" {
		if acc.FBUserID != "" && acc.FBUserID != snap.FBUserID {
			return c.Status(409).JSON(fiber.Map{
				"error": "this browser profile is logged into a different Facebook user; use a separate account slot for multi-account automation",
			})
		}
		if err := h.persistFacebookBrowserSession(id, orgID, inst, snap.FBUserID, snap.cookiesJSON); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		snap.LoggedIn = true
		snap.StoredFBID = snap.FBUserID
	}

	if snap.HumanRequired {
		h.recordWorkspaceHumanRequired(id, orgID, inst, snap)
		return c.JSON(snap)
	}

	return c.JSON(snap)
}

func (h *Handler) workspaceSetLoggedIn(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	orgID, _ := c.Locals("org_id").(int64)
	acc, err := h.db.Identities().GetAccountForOrg(id, orgID)
	if err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	var body struct {
		LoggedIn bool `json:"logged_in"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	var fbUserID string
	var cookiesJSON string
	if body.LoggedIn {
		if h.workspace == nil {
			return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
		}
		inst := h.workspaceInstanceForAccount(id, acc.Name)
		if inst == nil || inst.CDPPort == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "browser is not running"})
		}
		var cookieCount int
		var err error
		fbUserID, cookiesJSON, cookieCount, err = facebookCookiesFromInstance(inst)
		if err != nil {
			if isCDPUnavailable(err) {
				msg := "CDP endpoint is not reachable from the API host; restart this browser session after the thg-browser image is rebuilt"
				h.recordBrowserSession(id, acc.OrgID, inst, "display_ready", msg+": "+err.Error())
				return c.Status(503).JSON(fiber.Map{
					"error": "browser CDP is not ready: " + err.Error(),
					"code":  "CDP_UNAVAILABLE",
					"hint":  msg,
				})
			}
			return c.Status(400).JSON(fiber.Map{"error": "not logged in to Facebook yet: " + err.Error()})
		}
		_ = cookieCount
		if acc.FBUserID != "" && acc.FBUserID != fbUserID {
			return c.Status(409).JSON(fiber.Map{
				"error": "this account profile is logged into a different Facebook user; create another Facebook account slot for multi-account automation, or clear this profile before reusing it",
			})
		}
	}

	if body.LoggedIn {
		if err := h.persistFacebookBrowserSession(id, orgID, h.workspaceInstanceForAccount(id, acc.Name), fbUserID, cookiesJSON); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	} else if err := h.db.Identities().SetBrowserLoggedIn(id, false); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "logged_in": body.LoggedIn, "fb_user_id": fbUserID})
}

func (h *Handler) resolveCheckpoint(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	orgID, _ := c.Locals("org_id").(int64)
	if orgID != 0 {
		if acc, err := h.db.Identities().GetAccountForOrg(id, orgID); err != nil || acc == nil {
			return c.Status(404).JSON(fiber.Map{"error": "account not found"})
		}
	}
	sm := session.NewStateMachine(h.db.DB())
	cm := session.NewCheckpointManager(h.db.DB(), sm, nil)
	cm.SetVerifier(h.checkpointVerifier())
	if err := cm.ResolveCheckpoint(c.Context(), id); err != nil {
		// 409 when the browser still reports a verification page so the
		// frontend can keep the operator on the VNC view instead of
		// silently flipping back to the dashboard list.
		if _, still := err.(*session.ErrCheckpointStillActive); still {
			return c.Status(409).JSON(fiber.Map{
				"error": err.Error(),
				"code":  "CHECKPOINT_STILL_ACTIVE",
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "account_id": id, "status": "ready"})
}

// checkpointVerifier returns a session.CheckpointVerifier backed by the
// workspace manager's CDP probe. It returns nil when the workspace is
// not initialised so the operator-trust fallback path stays usable.
func (h *Handler) listCheckpoints(c *fiber.Ctx) error {
	sm := session.NewStateMachine(h.db.DB())
	cm := session.NewCheckpointManager(h.db.DB(), sm, nil)
	pending, err := cm.PendingCheckpoints(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"checkpoints": pending, "count": len(pending)})
}

func (h *Handler) getAccounts(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	// RBAC-1 (Organic Sales Network): execution-layer ownership scoping —
	// sales staff see ONLY accounts assigned to them; admin / platform see
	// every account in the org. Mirrors workspaceList; leads stay shared
	// elsewhere (feedback_shared_battlefield_not_crm). Previously this returned
	// GetAllAccounts to every role — a cross-member account leak.
	var (
		accounts []models.Account
		err      error
	)
	if r := models.UserRole(role); models.IsPlatformRole(r) || r == models.RoleAdmin {
		accounts, err = h.db.Identities().GetAllAccounts(orgID)
	} else {
		accounts, err = h.db.Identities().GetAccountsForUser(orgID, userID)
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	for i := range accounts {
		accounts[i].CookiesJSON = "[REDACTED]"
	}
	return c.JSON(fiber.Map{"accounts": accounts, "count": len(accounts)})
}

func (h *Handler) addAccount(c *fiber.Ctx) error {
	var req struct {
		Platform    string `json:"platform"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		CookiesJSON string `json:"cookies_json"` // optional ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â staff uses Chrome login instead
		ProxyURL    string `json:"proxy_url"`
		Notes       string `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	if req.Platform == "" {
		req.Platform = "facebook"
	}

	userID, _ := c.Locals("user_id").(int64)
	orgID, _ := c.Locals("org_id").(int64)

	acc := &models.Account{
		OrgID:          orgID,
		Platform:       models.Platform(req.Platform),
		Name:           req.Name,
		Email:          req.Email,
		CookiesJSON:    req.CookiesJSON,
		ProxyURL:       req.ProxyURL,
		Status:         models.AccountInactive,
		Notes:          req.Notes,
		AssignedUserID: userID,
	}
	// If cookies provided upfront, mark active immediately
	if req.CookiesJSON != "" {
		acc.Status = models.AccountActive
	}

	id, err := h.db.Identities().AddAccount(acc)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	log.Printf("[Accounts] Account created: %s (id=%d) by user %d", req.Name, id, userID)
	return c.Status(201).JSON(fiber.Map{"account_id": id})
}

func (h *Handler) updateAccountStatus(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	orgID, _ := c.Locals("org_id").(int64)
	acc, err := h.db.Identities().GetAccountForOrg(id, orgID)
	if err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}

	if err := h.db.Identities().UpdateAccountStatus(id, models.AccountStatus(req.Status)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

func (h *Handler) updateAccountCookies(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var req struct {
		CookiesJSON string `json:"cookies_json"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	orgID, _ := c.Locals("org_id").(int64)
	if acc, err := h.db.Identities().GetAccountForOrg(id, orgID); err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}

	if err := h.db.Identities().UpdateAccountCookies(id, req.CookiesJSON); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

func (h *Handler) deleteAccount(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	orgID, _ := c.Locals("org_id").(int64)
	if acc, err := h.db.Identities().GetAccountForOrg(id, orgID); err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}

	if err := h.db.Identities().DeleteAccount(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

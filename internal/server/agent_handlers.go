package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/store"
)

// agentAuth middleware validates X-Agent-Token header.
func (s *Server) agentAuth(c *fiber.Ctx) error {
	plain := c.Get("X-Agent-Token")
	if plain == "" {
		log.Printf("[AgentAuth] missing token path=%s ip=%s ua=%q", c.Path(), c.IP(), c.Get("User-Agent"))
		return c.Status(401).JSON(fiber.Map{"error": "missing X-Agent-Token"})
	}
	tok, err := s.db.ValidateAgentToken(plain)
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
	_ = s.db.UpdateAgentHeartbeat(tok.ID, c.Get("X-Agent-Hostname"), c.Get("X-Agent-OS"), c.Get("X-Agent-Version"))
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
func clampPresenceFields(p *store.AgentPresence) {
	p.Hostname = truncateRune(p.Hostname, limConnectorHostname)
	p.OS = truncateRune(p.OS, limConnectorOS)
	p.Version = truncateRune(p.Version, limConnectorVersion)
	p.Kind = truncateRune(p.Kind, limConnectorKind)
	p.Transport = truncateRune(p.Transport, limConnectorTransport)
	p.CapabilitiesJSON = truncateRune(p.CapabilitiesJSON, limConnectorCapabilitiesLen)
	p.CurrentURL = truncateRune(p.CurrentURL, limConnectorURL)
	p.FBUserID = truncateRune(p.FBUserID, limConnectorFBUserID)
	p.FBDisplayName = truncateRune(p.FBDisplayName, limConnectorFBName)
	p.FBUsername = truncateRune(p.FBUsername, limConnectorFBUsername)
	p.FBProfileURL = truncateRune(p.FBProfileURL, limConnectorFBProfileURL)
	p.StreamStatus = truncateRune(p.StreamStatus, limConnectorStreamStatus)
	p.ChromeError = truncateRune(p.ChromeError, limConnectorChromeError)
}

// agentGetOutbox returns approved outbound messages for local execution.
// GET /api/agent/outbox
func (s *Server) agentGetOutbox(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	agentID, _ := c.Locals("agent_id").(int64)
	assignedAccountID, _ := c.Locals("agent_assigned_account_id").(int64)
	workerID, _ := c.Locals("agent_token_fp").(string)
	if orgID <= 0 || agentID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	limit := c.QueryInt("limit", 5)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	_ = s.db.ResetStaleSendingOutboundForOrg(orgID, 10*time.Minute)
	candidates, err := s.db.GetOutboundByStatusForOrg(orgID, string(models.OutboundApproved), limit*4)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	msgs := make([]models.OutboundMessage, 0, limit)
	for _, msg := range candidates {
		if len(msgs) >= limit {
			break
		}
		if msg.AccountID <= 0 {
			continue
		}
		if assignedAccountID > 0 && msg.AccountID != assignedAccountID {
			continue
		}
		ownsStream, err := s.db.ConnectorOwnsAccountStream(orgID, agentID, msg.AccountID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if !ownsStream {
			continue
		}
		if err := s.db.ClaimApprovedOutboundForOrg(orgID, msg.ID, workerID); err != nil {
			continue
		}
		msg.Status = models.OutboundSending
		msgs = append(msgs, msg)
	}
	return c.JSON(fiber.Map{"messages": msgs, "count": len(msgs)})
}

// agentOutboxSent marks an outbound message as sent (agent executed it successfully).
// POST /api/agent/outbox/:id/sent
func (s *Server) agentOutboxSent(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundSent); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	if msg, err := s.db.GetOutboundForOrg(orgID, id); err == nil && msg.Type == "inbox" && msg.TargetURL != "" {
		threadID, threadErr := s.db.CreateThreadForOrg(orgID, 0, string(msg.Platform), msg.TargetURL, msg.TargetName, "")
		if threadErr == nil {
			_ = s.db.AddThreadMessage(threadID, "outbound", msg.Content, true)
		}
	}
	s.notifyOutboundStatus(orgID, id, models.OutboundSent)
	return c.JSON(fiber.Map{"status": "sent"})
}

// agentOutboxFailed marks an outbound message as failed.
// POST /api/agent/outbox/:id/failed
func (s *Server) agentOutboxFailed(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundFailed); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	s.notifyOutboundStatus(orgID, id, models.OutboundFailed)
	return c.JSON(fiber.Map{"status": "failed"})
}

// agentCreateToken creates a new agent token (admin only, JWT auth).
// POST /api/admin/agent-tokens
func (s *Server) agentCreateToken(c *fiber.Ctx) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	userID, _ := c.Locals("user_id").(int64)
	orgID, _ := c.Locals("org_id").(int64)
	id, plain, err := s.db.CreateAgentToken(req.Name, userID, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[Agent] Token created: %q (id=%d) by user %d", req.Name, id, userID)
	return c.Status(201).JSON(fiber.Map{
		"id":    id,
		"name":  req.Name,
		"token": plain, // shown once; client must copy immediately
	})
}

// agentListTokens lists all agent tokens (admin only, JWT auth).
// GET /api/admin/agent-tokens
func (s *Server) agentListTokens(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	tokens, err := s.db.ListAgentTokens(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"tokens": tokens, "count": len(tokens)})
}

// agentRevokeToken deactivates an agent token (admin only, JWT auth).
// DELETE /api/admin/agent-tokens/:id
func (s *Server) agentRevokeToken(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.RevokeAgentToken(id, orgID); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "agent token not found"})
	}
	return c.JSON(fiber.Map{"status": "revoked"})
}

// agentServeImage serves a local image file for the agent to download (for comment attachments).
// GET /api/agent/images?path=data/images/xxx.jpg
func (s *Server) agentServeImage(c *fiber.Ctx) error {
	relPath := c.Query("path")
	if relPath == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path required"})
	}
	// Sanitize: only allow paths starting with data/images/
	if len(relPath) < 12 || relPath[:12] != "data/images/" {
		return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
	}
	return c.SendFile(relPath)
}

// agentHeartbeat is a lightweight ping for connection health checks.
// POST /api/agent/heartbeat
func (s *Server) agentHeartbeat(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	var body struct {
		Hostname         string `json:"hostname"`
		OS               string `json:"os"`
		Version          string `json:"version"`
		Kind             string `json:"kind"`
		Transport        string `json:"transport"`
		AccountID        int64  `json:"account_id"`
		CapabilitiesJSON string `json:"capabilities_json"`
		CurrentURL       string `json:"current_url"`
		FBUserID         string `json:"fb_user_id"`
		FBDisplayName    string `json:"fb_display_name"`
		FBUsername       string `json:"fb_username"`
		FBProfileURL     string `json:"fb_profile_url"`
		LoginEmail       string `json:"login_email"`
		StreamStatus     string `json:"stream_status"`
		ChromeError      string `json:"chrome_error"`
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
	presence := store.AgentPresence{
		Hostname:          body.Hostname,
		OS:                body.OS,
		Version:           body.Version,
		Kind:              body.Kind,
		Transport:         body.Transport,
		AssignedAccountID: body.AccountID,
		CapabilitiesJSON:  body.CapabilitiesJSON,
		CurrentURL:        body.CurrentURL,
		FBUserID:          body.FBUserID,
		FBDisplayName:     body.FBDisplayName,
		FBUsername:        body.FBUsername,
		FBProfileURL:      body.FBProfileURL,
		StreamStatus:      body.StreamStatus,
		ChromeError:       body.ChromeError,
	}
	clampPresenceFields(&presence)
	_ = s.db.UpdateAgentPresence(agentID, presence)
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
func (s *Server) agentChromeStatus(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	var body struct {
		AccountID     int64  `json:"account_id"`
		CurrentURL    string `json:"current_url"`
		FBUserID      string `json:"fb_user_id"`
		FBDisplayName string `json:"fb_display_name"`
		FBUsername    string `json:"fb_username"`
		FBProfileURL  string `json:"fb_profile_url"`
		LoginEmail    string `json:"login_email"`
		StreamStatus  string `json:"stream_status"`
		ChromeError   string `json:"chrome_error"`
	}
	_ = c.BodyParser(&body)

	status := strings.TrimSpace(body.StreamStatus)
	if status == "" {
		status = browsergateway.StreamChromeNotConnected
	}
	presence := store.AgentPresence{
		AssignedAccountID: body.AccountID,
		CurrentURL:        body.CurrentURL,
		FBUserID:          body.FBUserID,
		FBDisplayName:     body.FBDisplayName,
		FBUsername:        body.FBUsername,
		FBProfileURL:      body.FBProfileURL,
		StreamStatus:      status,
		ChromeError:       body.ChromeError,
	}
	clampPresenceFields(&presence)
	_ = s.db.UpdateAgentPresence(agentID, presence)
	if body.AccountID > 0 && orgID > 0 {
		acc, err := s.db.GetAccountForOrg(body.AccountID, orgID)
		if err != nil || acc == nil {
			return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
		}
		loggedIn := strings.EqualFold(status, browsergateway.StreamFacebookLoggedIn) && strings.TrimSpace(body.FBUserID) != ""
		if loggedIn {
			if errResp := s.rejectIfFacebookProfileMismatch(c, c.Context(), acc, body.FBUserID, orgID); errResp != nil {
				return errResp
			}
		}
		if err := s.applyConnectorIdentity(c.Context(), connectorIdentitySnapshot{
			AccountID:     body.AccountID,
			OrgID:         orgID,
			StreamStatus:  status,
			CurrentURL:    body.CurrentURL,
			FBUserID:      body.FBUserID,
			FBDisplayName: body.FBDisplayName,
			FBUsername:    body.FBUsername,
			FBProfileURL:  body.FBProfileURL,
			LoginEmail:    body.LoginEmail,
			ChromeError:   body.ChromeError,
		}); err != nil {
			return c.Status(409).JSON(fiber.Map{"error": err.Error()})
		}
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
func (s *Server) agentBrowserTargets(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	agentID, _ := c.Locals("agent_id").(int64)
	createdBy, _ := c.Locals("agent_created_by").(int64)
	assignedAccountID, _ := c.Locals("agent_assigned_account_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	if _, err := store.NewAppStore(s.db); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	targets, err := s.db.ListLocalBrowserTargetsForConnector(orgID, agentID, createdBy, assignedAccountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if len(targets) == 0 && assignedAccountID > 0 {
		if err := s.ensureAssignedLocalBrowserTarget(c.Context(), orgID, assignedAccountID); err != nil {
			log.Printf("[AgentTargets] auto-bootstrap failed org_id=%d agent_id=%d account_id=%d: %v",
				orgID, agentID, assignedAccountID, err)
		} else if refreshed, err := s.db.ListLocalBrowserTargetsForConnector(orgID, agentID, createdBy, assignedAccountID); err == nil {
			targets = refreshed
		}
	}
	resp := fiber.Map{
		"targets":             targets,
		"count":               len(targets),
		"assigned_account_id": assignedAccountID,
	}
	if len(targets) == 0 {
		resp["hint_code"], resp["hint"] = browserTargetsHint(s, orgID, assignedAccountID)
	}
	return c.JSON(resp)
}

// browserTargetsHint inspects the org's account state to explain why the
// connector has no runnable browser target yet. The short code is stable for
// Chrome Extension clients; the prose is a fallback for older clients.
func browserTargetsHint(s *Server, orgID, assignedAccountID int64) (string, string) {
	if orgID <= 0 {
		return "no_org", "Chrome Extension chưa được gắn vào workspace nào. Pair lại bằng mã mới từ Browser dashboard."
	}
	accounts, _ := s.db.GetAllAccounts(orgID)
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

// agentConnectorCommands returns pending dashboard input commands for this Chrome Extension.
// GET /api/connectors/commands
func (s *Server) agentConnectorCommands(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	limit := c.QueryInt("limit", 20)
	commands, err := s.db.ClaimPendingConnectorCommands(orgID, agentID, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"commands": commands, "count": len(commands)})
}

// agentConnectorCommandDone marks a dashboard input command as executed by this Chrome Extension.
// POST /api/connectors/commands/:id/done
func (s *Server) agentConnectorCommandDone(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid command id"})
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = c.BodyParser(&body)
	if err := s.db.CompleteConnectorCommand(id, agentID, body.Error); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "command not found"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

// agentConnectorCrawlResult stores crawl output produced by THG Chrome Extension.
// The extension runs inside the user's signed-in Chrome, so this is the
// production path for Facebook sessions that the server does not own directly.
// POST /api/connectors/crawl-result
func (s *Server) agentConnectorCrawlResult(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	var body struct {
		TaskID    string   `json:"task_id"`
		Intent    string   `json:"intent"`
		AccountID int64    `json:"account_id"`
		Status    string   `json:"status"`
		Error     string   `json:"error"`
		Keywords  []string `json:"keywords"`
		Items     []struct {
			ID               string `json:"id"`
			SourceURL        string `json:"source_url"`
			AuthorProfileURL string `json:"author_profile_url"`
			AuthorName       string `json:"author_name"`
			Content          string `json:"content"`
			Reactions        int    `json:"reactions"`
			Comments         int    `json:"comments"`
			Shares           int    `json:"shares"`
		} `json:"items"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	body.TaskID = strings.TrimSpace(body.TaskID)
	if body.TaskID == "" || body.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "task_id and account_id are required"})
	}
	if acc, err := s.db.GetAccountForOrg(body.AccountID, orgID); err != nil || acc == nil {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}
	ownsStream, err := s.db.ConnectorOwnsAccountStream(orgID, agentID, body.AccountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !ownsStream {
		return c.Status(403).JSON(fiber.Map{"error": "connector does not own this account stream"})
	}

	appStore, err := store.NewAppStore(s.db)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	intent := strings.TrimSpace(body.Intent)
	if intent == "" {
		intent = "facebook_crawl"
	}
	_ = appStore.CreateTask(c.Context(), body.TaskID, orgID, intent)
	_ = appStore.StartTask(c.Context(), body.TaskID)
	if strings.EqualFold(body.Status, "failed") || strings.TrimSpace(body.Error) != "" {
		errMsg := strings.TrimSpace(body.Error)
		if errMsg == "" {
			errMsg = "Chrome Extension crawl failed"
		}
		_ = appStore.FailTask(c.Context(), body.TaskID, errMsg)
		s.notifyCrawlFailure(orgID, body.AccountID, body.TaskID, errMsg)
		return c.JSON(fiber.Map{"status": "failed", "error": errMsg})
	}

	guidance := orgScoringGuidance(s.db, orgID)
	keywords := normalizeCrawlKeywords(append(body.Keywords, orgIntelligenceKeywords(s.db, orgID)...))
	inserted := 0
	fetched := 0
	primarySourceURL := ""
	for _, item := range body.Items {
		content := strings.TrimSpace(item.Content)
		if content == "" || len([]rune(content)) < 20 {
			continue
		}
		fetched++
		sourceURL := strings.TrimSpace(item.SourceURL)
		if sourceURL == "" {
			sourceURL = strings.TrimSpace(item.ID)
		}
		if primarySourceURL == "" && sourceURL != "" {
			primarySourceURL = sourceURL
		}
		score, category, signals := scoreConnectorCrawlItem(content, keywords, item.Reactions, item.Comments, item.Shares, guidance)
		if category == "cold" || category == "rejected" {
			continue
		}
		lead := store.TaskLead{
			TaskID:           body.TaskID,
			OrgID:            orgID,
			SourceURL:        sourceURL,
			AuthorProfileURL: strings.TrimSpace(item.AuthorProfileURL),
			AuthorName:       strings.TrimSpace(item.AuthorName),
			Content:          content,
			LeadScore:        score,
			Category:         category,
			Signals:          signals,
		}
		if err := appStore.InsertLead(c.Context(), body.TaskID, orgID, lead); err == nil {
			inserted++
		}
		if _, err := s.db.InsertLead(&models.Lead{
			OrgID:        orgID,
			SourceType:   "post",
			SourceID:     0,
			SourceURL:    sourceURL,
			Platform:     models.PlatformFacebook,
			Author:       lead.AuthorName,
			AuthorURL:    lead.AuthorProfileURL,
			Content:      content,
			Score:        models.LeadScore(category),
			ServiceMatch: category,
			AuthorRole:   string(browsergateway.ProviderChromeExtension) + " classifier",
			PainPoint:    strings.Join(signals, "; "),
			AIReasoning:  strings.Join(signals, "; "),
			Niche:        strings.Join(keywords, ", "),
			ClassifiedAt: time.Now().UTC(),
		}); err != nil {
			log.Printf("[ConnectorCrawl] insert legacy lead failed task=%s: %v", body.TaskID, err)
		}
	}
	_ = appStore.CompleteTask(c.Context(), body.TaskID, fetched, inserted)
	s.notifyCrawlSummary(orgID, body.AccountID, body.TaskID, intent, fetched, inserted, primarySourceURL)
	return c.JSON(fiber.Map{
		"status":   "stored",
		"task_id":  body.TaskID,
		"fetched":  fetched,
		"inserted": inserted,
	})
}

func normalizeCrawlKeywords(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func orgIntelligenceKeywords(db *store.Store, orgID int64) []string {
	if db == nil || orgID <= 0 {
		return nil
	}
	var combined strings.Builder
	for _, key := range []string{"business_profile", "business_industry", "services", "target_customers", "target_signals", "markets", "private_files_summary", "data_sources_summary"} {
		value, err := db.GetContext(fmt.Sprintf("org:%d:%s", orgID, key))
		if err == nil && strings.TrimSpace(value) != "" {
			combined.WriteByte(' ')
			combined.WriteString(value)
		}
	}
	text := strings.ToLower(combined.String())
	if text == "" {
		return nil
	}
	stop := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true, "that": true,
		"this": true, "you": true, "your": true, "are": true, "can": true, "will": true,
		"toi": true, "tôi": true, "cua": true, "của": true, "cho": true, "voi": true,
		"với": true, "cac": true, "các": true, "nhung": true, "những": true, "khach": true,
		"khách": true, "hang": true, "hàng": true,
	}
	seen := map[string]bool{}
	out := make([]string, 0, 24)
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r == '_' || r == '-' || r == '+' || r == '#' || r == '@' || r == '.' || r == '/' || r == ':' || r == '%' || r == '&' || r == '=' || r == '?' || r == '\'' || r == '"' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || r >= 128)
	}) {
		token = strings.Trim(token, "._-+/:%&=?'\"")
		if len([]rune(token)) < 3 || stop[token] || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
		if len(out) >= 24 {
			break
		}
	}
	return out
}

func orgScoringGuidance(db *store.Store, orgID int64) scoring.Guidance {
	if db == nil || orgID <= 0 {
		return scoring.Guidance{}
	}
	get := func(key string) string {
		value, _ := db.GetContext(fmt.Sprintf("org:%d:%s", orgID, key))
		return strings.TrimSpace(value)
	}
	return scoring.Guidance{
		TargetAuthorRole: get("target_author_role"),
		TargetSignals:    splitSignalPhrases(get("target_signals")),
		RejectPhrases:    splitSignalPhrases(strings.Join([]string{get("negative_signals"), get("reject_rules")}, "\n")),
	}
}

func splitSignalPhrases(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';' || r == '|'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func scoreConnectorCrawlItem(content string, keywords []string, reactions, comments, shares int, guidance scoring.Guidance) (float64, string, []string) {
	result := scoring.New(scoring.DefaultConfig()).ScoreWithGuidance(content, keywords, reactions, comments, "", guidance)
	score := result.Score
	category := result.Category
	signals := append([]string{"chrome_extension_crawl"}, result.Signals...)
	if shares > 0 {
		signals = append(signals, "engagement:shares")
	}
	return score, category, signals
}

// agentScreenshot stores the latest observable frame from the user's real Chrome.
// POST /api/agent/screenshot
func (s *Server) agentScreenshot(c *fiber.Ctx) error {
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

	acc, err := s.db.GetAccountForOrg(body.AccountID, orgID)
	if err != nil || acc == nil {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}
	if errResp := s.rejectIfFacebookProfileMismatch(c, c.Context(), acc, body.FBUserID, orgID); errResp != nil {
		return errResp
	}

	streamStatus := strings.TrimSpace(body.StreamStatus)
	if streamStatus == "" {
		streamStatus = browsergateway.StreamConnectorOnline
	}
	if err := s.db.UpsertConnectorScreenshot(agentID, orgID, body.AccountID, body.ImageData, body.CurrentURL, body.FBUserID, body.FBDisplayName, body.FBUsername, body.FBProfileURL, streamStatus, body.ChromeError); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	presence := store.AgentPresence{
		CurrentURL:    body.CurrentURL,
		FBUserID:      body.FBUserID,
		FBDisplayName: body.FBDisplayName,
		FBUsername:    body.FBUsername,
		FBProfileURL:  body.FBProfileURL,
		StreamStatus:  streamStatus,
		ChromeError:   body.ChromeError,
	}
	clampPresenceFields(&presence)
	_ = s.db.UpdateAgentPresence(agentID, presence)

	if err := s.applyConnectorIdentity(c.Context(), connectorIdentitySnapshot{
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
	}); err != nil {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "stored", "ts": time.Now().Unix()})
}

func normalizeFacebookLoginEmail(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 320 {
		return ""
	}
	if strings.ContainsAny(value, " \t\r\n") || !strings.Contains(value, "@") {
		return ""
	}
	return value
}

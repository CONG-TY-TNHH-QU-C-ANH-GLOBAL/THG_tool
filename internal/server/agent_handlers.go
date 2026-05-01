package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// PostProcessor is called after an agent submits scraped posts so the AI pipeline can classify them.
// Wired to orchestrator.ProcessAgentScrapedPosts in main.go.
type PostProcessor func(ctx context.Context, groupURL string, posts []models.Post)

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

// agentGetNextJob returns the oldest pending local job, or 204 if none.
// GET /api/agent/jobs/next
func (s *Server) agentGetNextJob(c *fiber.Ctx) error {
	job, err := s.db.GetNextLocalJob()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if job == nil {
		return c.SendStatus(204)
	}
	return c.JSON(job)
}

// agentClaimJob marks a local job as running so no other agent picks it up.
// POST /api/agent/jobs/:id/claim
func (s *Server) agentClaimJob(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.UpdateJobStatus(id, models.JobRunning, "", ""); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	agentName, _ := c.Locals("agent_name").(string)
	log.Printf("[Agent] Job %d claimed by agent %q", id, agentName)
	return c.JSON(fiber.Map{"status": "claimed"})
}

// agentJobDone processes results submitted by the local agent.
// POST /api/agent/jobs/:id/done
func (s *Server) agentJobDone(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var body struct {
		GroupURL string        `json:"group_url"`
		Posts    []models.Post `json:"posts"`
		Summary  string        `json:"summary"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	saved := 0
	if len(body.Posts) > 0 {
		saved, _ = s.db.InsertPostsBatch(body.Posts)
		log.Printf("[Agent] Job %d: received %d posts, saved %d new", id, len(body.Posts), saved)

		// Run AI classification pipeline asynchronously
		if s.postProcessor != nil {
			go s.postProcessor(context.Background(), body.GroupURL, body.Posts)
		}
	}

	result := body.Summary
	if result == "" {
		result = fmt.Sprintf(`{"posts_received":%d,"posts_saved":%d}`, len(body.Posts), saved)
	}
	_ = s.db.UpdateJobStatus(id, models.JobDone, result, "")
	return c.JSON(fiber.Map{"status": "done", "posts_saved": saved})
}

// agentJobFail records a job failure from the local agent.
// POST /api/agent/jobs/:id/fail
func (s *Server) agentJobFail(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = c.BodyParser(&body)
	_ = s.db.UpdateJobStatus(id, models.JobFailed, "", body.Error)
	log.Printf("[Agent] Job %d failed: %s", id, body.Error)
	return c.JSON(fiber.Map{"status": "failed"})
}

// agentGetOutbox returns approved outbound messages for local execution.
// GET /api/agent/outbox
func (s *Server) agentGetOutbox(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	msgs, err := s.db.GetOutboundByStatusForOrg(orgID, "approved", 5)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
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
		"token": plain, // shown once — client must copy immediately
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
		StreamStatus     string `json:"stream_status"`
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
	_ = s.db.UpdateAgentPresence(agentID, store.AgentPresence{
		Hostname:          body.Hostname,
		OS:                body.OS,
		Version:           body.Version,
		Kind:              body.Kind,
		Transport:         body.Transport,
		AssignedAccountID: body.AccountID,
		CapabilitiesJSON:  body.CapabilitiesJSON,
		CurrentURL:        body.CurrentURL,
		FBUserID:          body.FBUserID,
		StreamStatus:      body.StreamStatus,
	})
	return c.JSON(fiber.Map{
		"status":       "ok",
		"connector_id": agentID,
		"org_id":       orgID,
		"ts":           time.Now().Unix(),
	})
}

// agentChromeStatus is the explicit local Chrome handshake endpoint.
// The desktop connector calls this even before any workspace target exists, so
// the dashboard can distinguish "device paired" from "Chrome actually attached".
// POST /api/agent/chrome-status
func (s *Server) agentChromeStatus(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	var body struct {
		AccountID    int64  `json:"account_id"`
		CurrentURL   string `json:"current_url"`
		FBUserID     string `json:"fb_user_id"`
		StreamStatus string `json:"stream_status"`
	}
	_ = c.BodyParser(&body)

	status := strings.TrimSpace(body.StreamStatus)
	if status == "" {
		status = "chrome_not_connected"
	}
	_ = s.db.UpdateAgentPresence(agentID, store.AgentPresence{
		AssignedAccountID: body.AccountID,
		CurrentURL:        body.CurrentURL,
		FBUserID:          body.FBUserID,
		StreamStatus:      status,
	})
	if body.AccountID > 0 && orgID > 0 {
		if appStore, err := store.NewAppStore(s.db); err == nil {
			_ = appStore.UpsertSession(c.Context(), store.BrowserSession{
				AccountID:    body.AccountID,
				OrgID:        orgID,
				Status:       localSessionStatusFromStream(status),
				StartedAt:    time.Now().UTC(),
				LastActiveAt: time.Now().UTC(),
			})
		}
		if body.FBUserID != "" {
			_ = s.db.SetBrowserLoggedIn(body.AccountID, true, body.FBUserID)
			_ = s.db.UpdateAccountStatus(body.AccountID, models.AccountActive)
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
	return c.JSON(fiber.Map{"targets": targets, "count": len(targets)})
}

// agentConnectorCommands returns pending dashboard input commands for this local runtime.
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

// agentConnectorCommandDone marks a dashboard input command as executed by this runtime.
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

// agentScreenshot stores the latest observable frame from the user's real Chrome.
// POST /api/agent/screenshot
func (s *Server) agentScreenshot(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}

	var body struct {
		AccountID    int64  `json:"account_id"`
		ImageData    string `json:"image_data"`
		CurrentURL   string `json:"current_url"`
		FBUserID     string `json:"fb_user_id"`
		StreamStatus string `json:"stream_status"`
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

	acc, err := s.db.GetAccount(body.AccountID)
	if err != nil || acc == nil || acc.OrgID != orgID {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}
	if body.FBUserID != "" && acc.FBUserID != "" && acc.FBUserID != body.FBUserID {
		if appStore, err := store.NewAppStore(s.db); err == nil {
			_ = appStore.UpsertSession(c.Context(), store.BrowserSession{
				AccountID:    body.AccountID,
				OrgID:        orgID,
				Status:       "local_error",
				StartedAt:    time.Now().UTC(),
				LastActiveAt: time.Now().UTC(),
				ErrorMsg:     "Facebook profile mismatch; create a separate account slot for this Facebook user",
			})
		}
		return c.Status(409).JSON(fiber.Map{"error": "facebook profile mismatch for this account slot"})
	}

	streamStatus := strings.TrimSpace(body.StreamStatus)
	if streamStatus == "" {
		streamStatus = "connector_online"
	}
	if err := s.db.UpsertConnectorScreenshot(agentID, orgID, body.AccountID, body.ImageData, body.CurrentURL, body.FBUserID, streamStatus); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	_ = s.db.UpdateAgentPresence(agentID, store.AgentPresence{
		CurrentURL:   body.CurrentURL,
		FBUserID:     body.FBUserID,
		StreamStatus: streamStatus,
	})

	localStatus := localSessionStatusFromStream(streamStatus)
	if appStore, err := store.NewAppStore(s.db); err == nil {
		_ = appStore.UpsertSession(c.Context(), store.BrowserSession{
			AccountID:    body.AccountID,
			OrgID:        orgID,
			Status:       localStatus,
			StartedAt:    time.Now().UTC(),
			LastActiveAt: time.Now().UTC(),
			ErrorMsg:     "",
		})
	}
	if body.FBUserID != "" {
		_ = s.db.SetBrowserLoggedIn(body.AccountID, true, body.FBUserID)
		_ = s.db.UpdateAccountStatus(body.AccountID, models.AccountActive)
	}

	return c.JSON(fiber.Map{"status": "stored", "ts": time.Now().Unix()})
}

func localSessionStatusFromStream(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "facebook_logged_in":
		return "local_ready"
	case "facebook_human_required":
		return "local_human_required"
	case "facebook_login_required":
		return "local_login_required"
	case "chrome_not_connected":
		return "local_error"
	default:
		return "local_active"
	}
}

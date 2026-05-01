package server

import (
	"context"
	"fmt"
	"log"
	"strconv"
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
		return c.Status(401).JSON(fiber.Map{"error": "missing X-Agent-Token"})
	}
	tok, err := s.db.ValidateAgentToken(plain)
	if err != nil || tok == nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid or revoked agent token"})
	}
	c.Locals("agent_id", tok.ID)
	c.Locals("agent_org_id", tok.OrgID)
	c.Locals("agent_name", tok.Name)
	_ = s.db.UpdateAgentHeartbeat(tok.ID, c.Get("X-Agent-Hostname"), c.Get("X-Agent-OS"), c.Get("X-Agent-Version"))
	return c.Next()
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
	return c.JSON(fiber.Map{"ts": time.Now().Unix()})
}

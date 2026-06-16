package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store"
)

// parsePostedAtRFC3339 parses a Facebook post timestamp emitted by the
// extension. Empty / unparseable values return the zero time, which the
// ingest pipeline treats as degraded last-call-wins for cursor advancement.
func parsePostedAtRFC3339(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// agentConnectorCrawlResult stores crawl output produced by THG Chrome Extension.
// The extension runs inside the user's signed-in Chrome, so this is the
// production path for Facebook sessions that the server does not own directly.
// POST /api/connectors/crawl-result
//
// Thin edge adapter ONLY: parse + edge-validate, then delegate to the Fiber-free
// processConnectorCrawlResult (crawl_ingest.go) and map domain errors to HTTP status.
// All ingest logic lives in the processor — keep this handler boring.
func (h *Handler) agentConnectorCrawlResult(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	var req connectorCrawlResultRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	req.TaskID = strings.TrimSpace(req.TaskID)
	if req.TaskID == "" || req.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "task_id and account_id are required"})
	}

	res, err := h.processConnectorCrawlResult(c.Context(), agentID, orgID, req)
	if err != nil {
		switch {
		case errors.Is(err, errCrawlForbiddenAccount), errors.Is(err, errCrawlForbiddenStream):
			return c.Status(403).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if res.Status == "failed" {
		return c.JSON(fiber.Map{"status": "failed", "error": res.Error})
	}
	return c.JSON(fiber.Map{
		"status":   "stored",
		"task_id":  res.TaskID,
		"fetched":  res.Fetched,
		"inserted": res.Inserted,
	})
}

// agentConnectorCrawlProgress receives in-flight heartbeats from the Chrome
// Extension during a crawl so users on Telegram can see "X/N posts scraped"
// updates without waiting for the final summary. Rate-limited internally.
// POST /api/connectors/crawl-progress
func (h *Handler) agentConnectorCrawlProgress(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	var body struct {
		TaskID    string `json:"task_id"`
		Intent    string `json:"intent"`
		AccountID int64  `json:"account_id"`
		Stage     string `json:"stage"`
		Fetched   int    `json:"fetched"`
		Max       int    `json:"max"`
		SourceURL string `json:"source_url"`
		Done      bool   `json:"done"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	body.TaskID = strings.TrimSpace(body.TaskID)
	if body.TaskID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "task_id is required"})
	}
	if body.AccountID > 0 {
		if acc, err := h.db.Identities().GetAccountForOrg(body.AccountID, orgID); err != nil || acc == nil {
			return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
		}
	}
	if system.ShouldEmitCrawlProgress(body.TaskID, body.Fetched, body.Done) {
		system.NotifyCrawlProgress(h.db, h.notifier, orgID, body.AccountID, body.TaskID, body.Intent, body.Stage, body.Fetched, body.Max, body.SourceURL)
	}
	return c.JSON(fiber.Map{"status": "ok"})
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
		value, err := db.Leads().GetContext(fmt.Sprintf("org:%d:%s", orgID, key))
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
		value, _ := db.Leads().GetContext(fmt.Sprintf("org:%d:%s", orgID, key))
		return strings.TrimSpace(value)
	}
	return scoring.Guidance{
		TargetAuthorRole: get("target_author_role"),
		TargetSignals:    ai.SplitSignalPhrases(get("target_signals")),
		RejectPhrases:    ai.SplitSignalPhrases(strings.Join([]string{get("negative_signals"), get("reject_rules")}, "\n")),
	}
}

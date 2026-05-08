package agent

import (
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store"
)

// agentConnectorCrawlResult stores crawl output produced by THG Chrome Extension.
// The extension runs inside the user's signed-in Chrome, so this is the
// production path for Facebook sessions that the server does not own directly.
// POST /api/connectors/crawl-result
func (h *Handler) agentConnectorCrawlResult(c *fiber.Ctx) error {
	agentID, _ := c.Locals("agent_id").(int64)
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	var body struct {
		TaskID           string         `json:"task_id"`
		Intent           string         `json:"intent"`
		AccountID        int64          `json:"account_id"`
		Status           string         `json:"status"`
		Error            string         `json:"error"`
		ExitReason       string         `json:"exit_reason"`
		Keywords         []string       `json:"keywords"`
		MarketSignalGate map[string]any `json:"market_signal_gate"`
		UserPrompt       string         `json:"user_prompt"`
		Items            []struct {
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
	if acc, err := h.db.GetAccountForOrg(body.AccountID, orgID); err != nil || acc == nil {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}
	ownsStream, err := h.db.ConnectorOwnsAccountStream(orgID, agentID, body.AccountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !ownsStream {
		return c.Status(403).JSON(fiber.Map{"error": "connector does not own this account stream"})
	}

	appStore, err := store.NewAppStore(h.db)
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
		system.NotifyCrawlFailure(h.db, h.notifier, orgID, body.AccountID, body.TaskID, errMsg)
		return c.JSON(fiber.Map{"status": "failed", "error": errMsg})
	}

	guidance := orgScoringGuidance(h.db, orgID)
	keywords := normalizeCrawlKeywords(append(body.Keywords, orgIntelligenceKeywords(h.db, orgID)...))
	businessProfile := ai.LoadProfileForOrg(h.db, orgID)
	gate := leadingest.SignalGateFromMap(body.MarketSignalGate)
	var aiClass *ai.MessageGenerator
	if h.aiClass != nil {
		aiClass = h.aiClass()
	}
	deps := leadingest.Deps{
		AppStore:        appStore,
		LegacyDB:        h.db,
		Scorer:          scoring.New(scoring.DefaultConfig()),
		Guidance:        guidance,
		BusinessProfile: businessProfile,
		AIClass:         aiClass,
		SignalGate:      gate,
		Keywords:        keywords,
		UserPrompt:      strings.TrimSpace(body.UserPrompt),
		ExtraSignals:    []string{"chrome_extension_crawl"},
	}

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

		// 1. Deduplicate: Memory check before hitting AI.
		// Prevents bringing in duplicate leads and wasting expensive LLM tokens across multiple scrapes.
		if sourceURL != "" && appStore != nil {
			if exists, _ := appStore.HasLeadWithSourceURL(c.Context(), orgID, sourceURL); exists {
				continue
			}
		}

		outcome, err := leadingest.IngestPost(c.Context(), deps, leadingest.Input{
			TaskID:           body.TaskID,
			OrgID:            orgID,
			SourceURL:        sourceURL,
			AuthorName:       strings.TrimSpace(item.AuthorName),
			AuthorProfileURL: strings.TrimSpace(item.AuthorProfileURL),
			Content:          content,
			Reactions:        item.Reactions,
			Comments:         item.Comments,
			Shares:           item.Shares,
		})
		if err != nil {
			log.Printf("[ConnectorCrawl] ingest failed task=%s: %v", body.TaskID, err)
			continue
		}
		if outcome.Inserted {
			inserted++
		}
	}
	_ = appStore.CompleteTask(c.Context(), body.TaskID, fetched, inserted)
	system.NotifyCrawlSummary(h.db, h.notifier, orgID, body.AccountID, body.TaskID, intent, len(body.Items), fetched, inserted, primarySourceURL, body.ExitReason)
	return c.JSON(fiber.Map{
		"status":   "stored",
		"task_id":  body.TaskID,
		"fetched":  fetched,
		"inserted": inserted,
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
		if acc, err := h.db.GetAccountForOrg(body.AccountID, orgID); err != nil || acc == nil {
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
		TargetSignals:    ai.SplitSignalPhrases(get("target_signals")),
		RejectPhrases:    ai.SplitSignalPhrases(strings.Join([]string{get("negative_signals"), get("reject_rules")}, "\n")),
	}
}

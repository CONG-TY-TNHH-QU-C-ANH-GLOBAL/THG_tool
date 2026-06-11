package agent

import (
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
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
		IntentID         int64          `json:"intent_id"` // recurring crawl intent id; 0 for one-shot runs
		Status           string         `json:"status"`
		Error            string         `json:"error"`
		ExitReason       string         `json:"exit_reason"`
		ScrollDiag       map[string]any `json:"scroll_diag"` // PR-CRAWL1 forensic: passes / max_articles / scroll_moved_ever / ...
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
			// Routing/cursor fields emitted by the DOM crawler (extension).
			// Backward-compatible — older extensions that don't emit these
			// will leave them empty, and the server falls back to URL parsing.
			PostFBID  string `json:"post_fbid"`
			GroupFBID string `json:"group_fbid"`
			PostedAt  string `json:"posted_at"` // RFC3339; empty when crawler can't extract
		} `json:"items"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	body.TaskID = strings.TrimSpace(body.TaskID)
	if body.TaskID == "" || body.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "task_id and account_id are required"})
	}
	if acc, err := h.db.Identities().GetAccountForOrg(body.AccountID, orgID); err != nil || acc == nil {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}
	ownsStream, err := h.db.Connectors().ConnectorOwnsAccountStream(orgID, agentID, body.AccountID)
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
		IntentID:        body.IntentID,
		OnLeadCreated: func(ev leadingest.LeadEvent) {
			workspace := ""
			if org, _ := h.db.GetOrganization(ev.OrgID); org != nil {
				workspace = org.Name
			}
			h.tgEvents.NotifyLead(control.LeadNotice{
				OrgID: ev.OrgID, LeadID: ev.LeadID, Channel: "facebook", Workspace: workspace,
				Author: ev.AuthorName, PostURL: ev.PostURL, Excerpt: ev.Excerpt, Reason: ev.Reason, BaseURL: h.baseURL,
			})
		},
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
			SourceType:       "post",
			PrimaryURL:       sourceURL,
			PostFBID:         strings.TrimSpace(item.PostFBID),
			GroupFBID:        strings.TrimSpace(item.GroupFBID),
			PostedAt:         parsePostedAtRFC3339(item.PostedAt),
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
	// PR-CRAWL1 forensic: when a crawl yields suspiciously few raw posts, log the
	// scroll diagnostic so we can tell WHY without guessing. scroll_moved_ever=
	// false ⇒ our scroll never moved the feed (window minimized → rAF throttle, or
	// wrong scroll target); true with max_articles_seen≈1 ⇒ FB stopped loading
	// despite scrolling (platform side / genuinely few CHRONOLOGICAL posts).
	scrollNote := ""
	if len(body.Items) <= 2 && body.ScrollDiag != nil {
		sd := body.ScrollDiag
		scrollNote = fmt.Sprintf("moved=%v passes=%v max_articles=%v target=%v",
			sd["scroll_moved_ever"], sd["passes"], sd["max_articles_seen"], sd["final_scroll_target"])
		slog.WarnContext(c.Context(), "crawl-forensic: low yield",
			"org_id", orgID, "account_id", body.AccountID, "task_id", body.TaskID,
			"raw_items", len(body.Items), "exit_reason", body.ExitReason,
			"scroll_diag", body.ScrollDiag,
		)
	}
	system.NotifyCrawlSummary(h.db, h.notifier, orgID, body.AccountID, body.TaskID, intent, len(body.Items), fetched, inserted, primarySourceURL, body.ExitReason, scrollNote)
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

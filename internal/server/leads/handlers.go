package leads

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Transport-layer error messages, factored out to avoid duplicated string
// literals (go:S1192). Values are byte-identical to the originals.
const (
	errInvalidID      = "invalid id"
	errInvalidRequest = "invalid request"
)

// nicheSlugRe constrains niche slugs to a URL/filter-safe shape. Slugs flow
// into query params and lead-filter clauses, so reject anything outside
// lowercase alphanumerics, hyphen, and underscore.
var nicheSlugRe = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// Deps holds dependencies needed by the leads subpackage handlers.
//
// AIClass is a getter, not a value, because the parent server constructor
// registers routes BEFORE the universal classifier is wired (msgGen lives
// behind a SetUniversalClassifier setter). Capturing the field directly
// would freeze it at nil; the closure resolves the current value at
// request time.
type Deps struct {
	DB       *store.Store
	JobStore *jobs.Store
	AIClass  func() *ai.MessageGenerator
}

// getLeads handles GET /api/leads
func getLeads(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		score := c.Query("score", "")
		niche := c.Query("niche", "")
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		orgID, _ := c.Locals("org_id").(int64)

		var (
			leadList []models.Lead
			err      error
		)
		// The Chrome Extension crawl path stores into task_leads first, then best-effort
		// mirrors into legacy leads. Merge both tables for the main dashboard view so
		// production users immediately see extension-crawled results even when the
		// legacy mirror is empty or delayed.
		if niche == "" && offset == 0 {
			leadList, err = deps.DB.Leads().GetAutomationLeadsForOrg(orgID, score, limit)
		} else {
			leadList, err = deps.DB.Leads().GetLeadsFiltered(score, niche, limit, offset, orgID)
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"leads": leadList, "count": len(leadList)})
	}
}

// deleteLead handles DELETE /api/leads/:id?source=...
// `source` distinguishes which table the lead row lives in:
//   - "task_lead" → delete from task_leads (Chrome Extension crawl path)
//   - anything else (default) → delete from legacy leads
//
// Both paths also remove the mirror copy in the other table by source_url so
// the lead does not re-appear on the next dashboard refresh.
func deleteLead(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidID})
		}
		source := strings.ToLower(strings.TrimSpace(c.Query("source", "")))
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		if source == "task_lead" {
			if err := deps.DB.Leads().DeleteTaskLead(orgID, id); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.JSON(fiber.Map{"ok": true})
		}
		if err := deps.DB.Leads().DeleteLead(orgID, id); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

// reclassifyLeads handles POST /api/leads/reclassify (admin only).
//
// Re-runs the AI classifier over an org's existing leads using the new
// ClassifyIntent context (user prompt + target role + positive signals)
// so leads ingested before the classifier upgrade can be retagged
// without re-crawling. Defaults to leads whose author_role is unknown
// to avoid clobbering manually labelled rows.
//
// Body:
//
//	{
//	  "user_prompt":      "cào bài tuyển dụng nhân sự",
//	  "target_role":      "candidate",         // optional anchor for the AI
//	  "positive_signals": ["đang tuyển", ...], // optional anchor phrases
//	  "only_unknown":     true,                // default true; set false to retag everything
//	  "limit":            50                   // max 200, default 50
//	}
func reclassifyLeads(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
		}
		var classifier *ai.MessageGenerator
		if deps.AIClass != nil {
			classifier = deps.AIClass()
		}
		if classifier == nil || !classifier.Available() {
			return c.Status(503).JSON(fiber.Map{"error": "AI classifier is not configured (OPENAI_API_KEY missing)"})
		}

		var req struct {
			UserPrompt      string   `json:"user_prompt"`
			TargetRole      string   `json:"target_role"`
			PositiveSignals []string `json:"positive_signals"`
			OnlyUnknown     *bool    `json:"only_unknown"`
			Limit           int      `json:"limit"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidRequest})
		}
		onlyUnknown := true
		if req.OnlyUnknown != nil {
			onlyUnknown = *req.OnlyUnknown
		}
		limit := req.Limit
		if limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}

		profile := ai.LoadProfileForOrg(deps.DB, orgID)
		if profile == nil {
			profile = &ai.BusinessProfile{}
		}

		leadList, err := deps.DB.Leads().GetLeadsForReclassify(orgID, onlyUnknown, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if len(leadList) == 0 {
			return c.JSON(fiber.Map{
				"matched":      0,
				"reclassified": 0,
				"failed":       0,
				"message":      "Khong co lead nao thoa man dieu kien.",
			})
		}

		intent := ai.ClassifyIntent{
			UserPrompt:      strings.TrimSpace(req.UserPrompt),
			TargetRole:      strings.TrimSpace(req.TargetRole),
			PositiveSignals: req.PositiveSignals,
		}
		// Use the lead's stored niche as a keyword anchor when the caller
		// did not supply explicit positive signals — keeps classification
		// stable across mixed-vertical workspaces.
		fallbackKeywords := []string{}
		if len(intent.PositiveSignals) == 0 {
			fallbackKeywords = []string{strings.TrimSpace(profile.Industry)}
		}

		updated := 0
		failed := 0
		for _, lead := range leadList {
			content := strings.TrimSpace(lead.Content)
			if content == "" {
				failed++
				continue
			}
			perLeadIntent := intent
			if len(perLeadIntent.Keywords) == 0 {
				kw := append([]string{}, fallbackKeywords...)
				if niche := strings.TrimSpace(lead.Niche); niche != "" && niche != "logistics" {
					kw = append(kw, niche)
				}
				perLeadIntent.Keywords = kw
			}

			classifyCtx, cancel := context.WithTimeout(c.Context(), 20*time.Second)
			result, err := classifier.UniversalClassify(classifyCtx, content, lead.Author, profile, perLeadIntent)
			cancel()
			if err != nil || result == nil {
				log.Printf("[Reclassify] org=%d lead=%d failed: %v", orgID, lead.ID, err)
				failed++
				continue
			}
			score := strings.TrimSpace(result.Priority)
			if score == "" {
				score = string(lead.Score)
			}
			if score == "rejected" {
				score = "cold"
			}
			intentStr := strings.TrimSpace(result.Intent)
			if intentStr == "" {
				intentStr = "unknown"
			}
			painPoint := strings.TrimSpace(result.Reason)
			if painPoint == "" {
				painPoint = lead.PainPoint
			}
			if err := deps.DB.Leads().UpdateLeadClassification(orgID, lead.ID, score, intentStr, intentStr, painPoint, result.Reason); err != nil {
				log.Printf("[Reclassify] org=%d lead=%d update failed: %v", orgID, lead.ID, err)
				failed++
				continue
			}
			updated++
		}

		userID, _ := c.Locals("user_id").(int64)
		deps.DB.InsertAuditLog(userID, "leads_reclassified", c.IP(),
			fmt.Sprintf(`{"org_id":%d,"matched":%d,"reclassified":%d,"failed":%d,"only_unknown":%t}`,
				orgID, len(leadList), updated, failed, onlyUnknown))
		return c.JSON(fiber.Map{
			"matched":      len(leadList),
			"reclassified": updated,
			"failed":       failed,
		})
	}
}

// deleteAllLeads handles DELETE /api/leads/all
//
// Org-scoped: clears both the legacy `leads` mirror and the connector
// `task_leads` table for the caller's tenant only. Previously this used
// the global DeleteLeads which wiped every tenant's leads — a multi-tenant
// data-loss bug. Optional ?niche= narrows the legacy side.
func deleteAllLeads(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		niche := c.Query("niche", "")
		// Validate against the slug allowlist before the value reaches the
		// delete filter, the audit log, or the application log. Rejecting
		// anything outside [a-z0-9_-] also strips CR/LF, so user input can
		// no longer forge log records (sonar gosecurity:S5145).
		if niche != "" && !nicheSlugRe.MatchString(niche) {
			return c.Status(400).JSON(fiber.Map{"error": "invalid niche: use lowercase letters, digits, '-' or '_' (max 64)"})
		}
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		count, err := deps.DB.Leads().DeleteAllLeadsForOrg(orgID, niche)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		scope := "all"
		if niche != "" {
			scope = niche
		}
		userID, _ := c.Locals("user_id").(int64)
		deps.DB.InsertAuditLog(userID, "delete_leads", c.IP(), fmt.Sprintf(`{"scope":%q,"count":%d,"org_id":%d}`, scope, count, orgID))
		log.Printf("[API] Deleted leads (org=%d scope=%s): %d removed", orgID, scope, count)
		return c.JSON(fiber.Map{"ok": true, "deleted": count, "scope": scope})
	}
}

// getClassificationsRecent handles GET /api/leads/classifications/recent
// Admin-only diagnostic surface — answers "why did the AI reject every
// post in my crawl?". Filters: ?task_id=... (single batch),
// ?decision=kept|rejected|cold|error, ?limit=N (capped at 500).
func getClassificationsRecent(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		taskID := strings.TrimSpace(c.Query("task_id", ""))
		decision := strings.TrimSpace(c.Query("decision", ""))
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		entries, err := deps.DB.Leads().ListRecentClassifications(c.Context(), orgID, taskID, decision, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		// If filtering by task_id, also surface the rejection-reason breakdown.
		var breakdown any
		if taskID != "" {
			if b, err := deps.DB.Leads().SummariseClassifications(c.Context(), orgID, taskID); err == nil {
				breakdown = b
			}
		}
		return c.JSON(fiber.Map{
			"classifications": entries,
			"count":           len(entries),
			"breakdown":       breakdown,
		})
	}
}

// getNiches handles GET /api/niches
func getNiches(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		niches, err := deps.DB.Leads().GetNiches()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"niches": niches, "count": len(niches)})
	}
}

// addNiche handles POST /api/niches
func addNiche(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			Slug  string `json:"slug"`
			Name  string `json:"name"`
			Emoji string `json:"emoji"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidRequest})
		}
		req.Slug = strings.TrimSpace(req.Slug)
		req.Name = strings.TrimSpace(req.Name)
		if req.Slug == "" || req.Name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "slug and name required"})
		}
		if !nicheSlugRe.MatchString(req.Slug) {
			return c.Status(400).JSON(fiber.Map{"error": "invalid slug: use lowercase letters, digits, '-' or '_' (max 64)"})
		}
		if len(req.Name) > 120 {
			return c.Status(400).JSON(fiber.Map{"error": "name too long (max 120)"})
		}
		n := &models.Niche{Slug: req.Slug, Name: req.Name, Emoji: req.Emoji}
		id, err := deps.DB.Leads().InsertNiche(n)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"id": id, "slug": req.Slug})
	}
}

// deleteNiche handles DELETE /api/niches/:slug
func deleteNiche(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		slug := c.Params("slug")
		if slug == "logistics" {
			return c.Status(400).JSON(fiber.Map{"error": "cannot delete default niche"})
		}
		if err := deps.DB.Leads().DeleteNiche(slug); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "deleted"})
	}
}

// getPosts handles GET /api/posts
func getPosts(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		orgID, _ := c.Locals("org_id").(int64)
		posts, err := deps.DB.Crawl().GetRecentPosts(limit, offset, orgID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"posts": posts, "count": len(posts)})
	}
}

// deletePost handles DELETE /api/posts/:id
func deletePost(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidID})
		}
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		if err := deps.DB.Crawl().DeletePost(orgID, id); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

// deleteAllPosts handles DELETE /api/posts/all
//
// Org-scoped: clears only the caller's tenant posts (derived from owned
// groups). Previously this used the global DeleteAllPosts which wiped every
// tenant's posts — a multi-tenant data-loss bug, same class as the old
// DeleteAllLeads issue.
func deleteAllPosts(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		count, err := deps.DB.Crawl().DeleteAllPostsForOrg(orgID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		userID, _ := c.Locals("user_id").(int64)
		deps.DB.InsertAuditLog(userID, "delete_posts", c.IP(), fmt.Sprintf(`{"count":%d,"org_id":%d}`, count, orgID))
		log.Printf("[API] Deleted all posts (org=%d): %d removed", orgID, count)
		return c.JSON(fiber.Map{"ok": true, "deleted": count})
	}
}

// getGroups handles GET /api/groups
func getGroups(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		groups, err := deps.DB.Crawl().GetAllGroups(orgID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"groups": groups, "count": len(groups)})
	}
}

// addGroup handles POST /api/groups
func addGroup(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			Platform string `json:"platform"`
			Name     string `json:"name"`
			URL      string `json:"url"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidRequest})
		}
		req.Name = strings.TrimSpace(req.Name)
		req.URL = strings.TrimSpace(req.URL)
		if req.URL == "" {
			return c.Status(400).JSON(fiber.Map{"error": "url required"})
		}
		u, err := url.Parse(req.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return c.Status(400).JSON(fiber.Map{"error": "invalid url: must be an http(s) URL"})
		}
		if len(req.Name) > 200 {
			return c.Status(400).JSON(fiber.Map{"error": "name too long (max 200)"})
		}
		groupOrgID, _ := c.Locals("org_id").(int64)
		if groupOrgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		group := &models.Group{
			OrgID:     groupOrgID,
			Platform:  models.Platform(req.Platform),
			Name:      req.Name,
			URL:       req.URL,
			Active:    true,
			JoinState: "none",
		}
		id, err := deps.DB.Crawl().AddGroup(group)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"group_id": id})
	}
}

// toggleGroup handles PUT /api/groups/:id/toggle
func toggleGroup(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidID})
		}
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		var req struct {
			Active bool `json:"active"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidRequest})
		}
		if err := deps.DB.Crawl().ToggleGroup(orgID, id, req.Active); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "updated"})
	}
}

// deleteGroup handles DELETE /api/groups/:id
func deleteGroup(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidID})
		}
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		if err := deps.DB.Crawl().DeleteGroup(orgID, id); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "deleted"})
	}
}

// getJobs handles GET /api/jobs
func getJobs(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		status := c.Query("status", "")
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		list, err := deps.JobStore.List(c.Context(), status, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"jobs": list, "count": len(list)})
	}
}

// createJob handles POST /api/jobs
func createJob(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			Intent    string `json:"intent"`
			Platform  string `json:"platform"`
			Target    string `json:"target"`
			AccountID int64  `json:"account_id"`
			Text      string `json:"text"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidRequest})
		}
		if req.Intent == "" {
			req.Intent = "facebook_crawl"
		}
		if req.Target == "" {
			return c.Status(400).JSON(fiber.Map{"error": "target URL is required for crawler jobs; use /api/ai/prompt for free-form agent prompts"})
		}
		if req.Platform == "" {
			req.Platform = "facebook"
		}
		sourceType := req.Platform + "_group"
		if req.Platform == "web" || req.Platform == "website" {
			sourceType = "web_url"
		}
		task := &jobs.Task{
			SchemaVersion: "1",
			TaskID:        fmt.Sprintf("api-%s-%d", req.Intent, time.Now().UnixMilli()),
			AccountID:     req.AccountID,
			Intent:        req.Intent,
			Keywords:      strings.Fields(req.Text),
			CrawlPlan: jobs.CrawlPlan{
				Sources:  []jobs.Source{{Type: sourceType, URL: req.Target}},
				MaxItems: 50,
			},
			OutputSchema:        "open_crawler_v1",
			OutputSchemaVersion: "1",
		}
		task.OrgID, _ = c.Locals("org_id").(int64)
		payload, _ := json.Marshal(task)
		j, err := deps.JobStore.Submit(c.Context(), task, string(payload))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"job_id": j.ID, "task_id": j.TaskID, "status": "submitted"})
	}
}

// cancelJob handles DELETE /api/jobs/:id
func cancelJob(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid job id"})
		}
		if err := deps.JobStore.Cancel(c.Context(), id); err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "canceled"})
	}
}

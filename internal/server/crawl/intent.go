package crawl

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// Deps holds dependencies needed by crawl intent handlers.
type Deps struct {
	DB *store.Store
}

// maxActiveIntentsPerOrg caps how many recurring intents an org may keep in
// status='active' concurrently. Anything beyond this returns 429 — the org
// must pause/archive an existing intent before adding more.
const maxActiveIntentsPerOrg = 20

// minPromptLength is the floor for prompt text. Below this, classifier and
// keyword fallback do not produce useful signals.
const minPromptLength = 20

// facebookURLRe matches the surface set we currently know how to crawl:
// facebook.com group / page / profile URLs. We accept www. and https/http.
var facebookURLRe = regexp.MustCompile(`(?i)^https?://(www\.|m\.)?facebook\.com/`)

func listIntents(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		intents, err := deps.DB.ListCrawlIntentsForOrg(c.Context(), orgID, 100)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"intents": intents, "count": len(intents)})
	}
}

func setIntentEnabled(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil || id <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "invalid crawl intent id"})
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
		}
		if err := deps.DB.SetCrawlIntentEnabled(c.Context(), orgID, id, body.Enabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return c.Status(404).JSON(fiber.Map{"error": "crawl intent not found"})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "ok", "enabled": body.Enabled})
	}
}

// createIntentBody is the user-facing shape for POST /crawl-intents. Most
// fields are optional and fall back to store defaults — only prompt and
// source_url are required from the FE.
type createIntentBody struct {
	Prompt          string   `json:"prompt"`
	SourceURL       string   `json:"source_url"`
	Name            string   `json:"name"`
	IntervalMinutes int      `json:"interval_minutes"`
	MaxItems        int      `json:"max_items"`
	Keywords        []string `json:"keywords"`
	AccountID       int64    `json:"account_id"`
}

func createIntent(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(401).JSON(fiber.Map{"error": "org context required"})
		}
		var body createIntentBody
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
		}
		body.Prompt = strings.TrimSpace(body.Prompt)
		body.SourceURL = strings.TrimSpace(body.SourceURL)
		if len(body.Prompt) < minPromptLength {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("prompt phải có ít nhất %d ký tự", minPromptLength)})
		}
		if !facebookURLRe.MatchString(body.SourceURL) {
			return c.Status(400).JSON(fiber.Map{"error": "source_url phải là một URL facebook.com hợp lệ"})
		}

		active, err := deps.DB.CountActiveCrawlIntentsForOrg(c.Context(), orgID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if active >= maxActiveIntentsPerOrg {
			return c.Status(429).JSON(fiber.Map{
				"error": fmt.Sprintf("đã đạt giới hạn %d nhiệm vụ đang chạy — tạm dừng bớt trước khi tạo mới", maxActiveIntentsPerOrg),
			})
		}

		sourceType := "facebook_group"
		if !strings.Contains(strings.ToLower(body.SourceURL), "/groups/") {
			sourceType = "facebook_page"
		}

		// Pin next_run_at to "now" so the scheduler picks the intent up on
		// the next tick (≤1 min) instead of waiting a full interval — gives
		// users instant feedback after submitting the form. Re-normalize
		// inside UpsertCrawlIntent only fires when NextRunAt is zero.
		intent, err := deps.DB.UpsertCrawlIntent(c.Context(), store.CrawlIntent{
			OrgID:           orgID,
			AccountID:       body.AccountID,
			Name:            body.Name,
			Prompt:          body.Prompt,
			Intent:          "facebook_crawl",
			SourceType:      sourceType,
			SourceURL:       body.SourceURL,
			Keywords:        body.Keywords,
			IntervalMinutes: body.IntervalMinutes,
			MaxItems:        body.MaxItems,
			NextRunAt:       time.Now().UTC(),
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		// "Created" = the upsert inserted a new row. On INSERT, created_at
		// and updated_at are both CURRENT_TIMESTAMP from the same statement
		// so their delta is sub-second; on UPDATE, created_at predates
		// updated_at by at least the row's age. 2-second tolerance covers
		// clock skew on slow CI runners.
		created := intent.UpdatedAt.Sub(intent.CreatedAt) < 2*time.Second

		// Archived is sticky inside UpsertCrawlIntent — re-POSTing the same
		// URL on an archived row keeps it archived. We treat re-submission
		// as explicit user intent to reactivate.
		if intent.Status == store.CrawlIntentStatusArchived {
			if err := deps.DB.SetCrawlIntentStatus(c.Context(), orgID, intent.ID, store.CrawlIntentStatusActive); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			intent.Status = store.CrawlIntentStatusActive
			intent.Enabled = true
		}

		return c.JSON(fiber.Map{"intent": intent, "created": created})
	}
}

// transitionIntent is the explicit state-transition handler shared by the
// /pause, /resume, /archive endpoints. The target status is curried by the
// caller; the body is empty.
func transitionIntent(deps Deps, targetStatus string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil || id <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "invalid crawl intent id"})
		}
		if err := deps.DB.SetCrawlIntentStatus(c.Context(), orgID, id, targetStatus); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return c.Status(404).JSON(fiber.Map{"error": "crawl intent not found"})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "ok", "intent_status": targetStatus})
	}
}

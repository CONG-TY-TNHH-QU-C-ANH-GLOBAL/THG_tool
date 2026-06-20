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
	crawlstore "github.com/thg/scraper/internal/store/crawl"
)

// Transport-layer error messages, factored out to avoid duplicated string
// literals (go:S1192). Values are byte-identical to the originals.
const (
	errInvalidIntentID = "invalid crawl intent id"
	errInvalidRequest  = "invalid request"
	errIntentNotFound  = "crawl intent not found"
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
		intents, err := deps.DB.Crawl().ListIntentsForOrg(c.Context(), orgID, 100)
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
			return c.Status(400).JSON(fiber.Map{"error": errInvalidIntentID})
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidRequest})
		}
		if err := deps.DB.Crawl().SetIntentEnabled(c.Context(), orgID, id, body.Enabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return c.Status(404).JSON(fiber.Map{"error": errIntentNotFound})
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
			return c.Status(400).JSON(fiber.Map{"error": errInvalidRequest})
		}
		body.Prompt = strings.TrimSpace(body.Prompt)
		body.SourceURL = strings.TrimSpace(body.SourceURL)
		if len(body.Prompt) < minPromptLength {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("prompt phải có ít nhất %d ký tự", minPromptLength)})
		}
		if !facebookURLRe.MatchString(body.SourceURL) {
			return c.Status(400).JSON(fiber.Map{"error": "source_url phải là một URL facebook.com hợp lệ"})
		}

		active, err := deps.DB.Crawl().CountActiveIntentsForOrg(c.Context(), orgID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if active >= maxActiveIntentsPerOrg {
			return c.Status(429).JSON(fiber.Map{
				"error": fmt.Sprintf("đã đạt giới hạn %d nhiệm vụ đang chạy — tạm dừng bớt trước khi tạo mới", maxActiveIntentsPerOrg),
			})
		}

		// Mission Preflight (Reliability Track PR-A): a user-created crawl mission
		// MUST name a ready account. No account_id=0, no silent fallback to a
		// "first ready" account. Fail early with a typed, actionable reason.
		userID, _ := c.Locals("user_id").(int64)
		role, _ := c.Locals("user_role").(string)
		if reason, msg := EvaluateCrawlAccountReadiness(c.Context(), deps.DB, orgID, userID, role, body.AccountID); reason != ReadinessReady {
			return c.Status(422).JSON(fiber.Map{"error": msg, "reason_code": reason})
		}

		sourceType := "facebook_group"
		if !strings.Contains(strings.ToLower(body.SourceURL), "/groups/") {
			sourceType = "facebook_page"
		}

		// Pin next_run_at to "now" so the scheduler picks the intent up on
		// the next tick (≤1 min) instead of waiting a full interval — gives
		// users instant feedback after submitting the form. Re-normalize
		// inside UpsertCrawlIntent only fires when NextRunAt is zero.
		intent, err := deps.DB.Crawl().UpsertIntent(c.Context(), crawlstore.Intent{
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
		if intent.Status == crawlstore.IntentStatusArchived {
			if err := deps.DB.Crawl().SetIntentStatus(c.Context(), orgID, intent.ID, crawlstore.IntentStatusActive); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			intent.Status = crawlstore.IntentStatusActive
			intent.Enabled = true
		}

		return c.JSON(fiber.Map{"intent": intent, "created": created})
	}
}

// setIntentInterval handles PATCH /crawl-intents/:id/interval. Body:
// {"interval_minutes": N}. Bounds are enforced inside the store.
func setIntentInterval(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil || id <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidIntentID})
		}
		var body struct {
			IntervalMinutes int `json:"interval_minutes"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidRequest})
		}
		if body.IntervalMinutes <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "interval_minutes phải > 0"})
		}
		if err := deps.DB.Crawl().SetIntentInterval(c.Context(), orgID, id, body.IntervalMinutes); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return c.Status(404).JSON(fiber.Map{"error": errIntentNotFound})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "ok", "interval_minutes": body.IntervalMinutes})
	}
}

// deleteIntent handles DELETE /crawl-intents/:id. Hard delete; the
// row is removed permanently. Distinct from /archive (soft).
func deleteIntent(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil || id <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": errInvalidIntentID})
		}
		if err := deps.DB.Crawl().DeleteIntent(c.Context(), orgID, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return c.Status(404).JSON(fiber.Map{"error": errIntentNotFound})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "ok", "deleted": true})
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
			return c.Status(400).JSON(fiber.Map{"error": errInvalidIntentID})
		}
		if err := deps.DB.Crawl().SetIntentStatus(c.Context(), orgID, id, targetStatus); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return c.Status(404).JSON(fiber.Map{"error": errIntentNotFound})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "ok", "intent_status": targetStatus})
	}
}

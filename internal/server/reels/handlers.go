package reels

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	reelsvc "github.com/thg/scraper/internal/services/reel"
	reelstore "github.com/thg/scraper/internal/store/reel"
)

func orgAndUser(c *fiber.Ctx) (int64, int64) {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	return orgID, userID
}

// reelID parses the :id param; returns ok=false after writing a 400.
func reelID(c *fiber.Ctx) (int64, bool) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		_ = c.Status(400).JSON(fiber.Map{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

// statusForErr maps a service error to an HTTP status (404 for not-found, else 500).
func statusForErr(err error) int {
	if errors.Is(err, reelstore.ErrReelNotFound) {
		return 404
	}
	return 500
}

// createReel handles POST /api/reels — create draft + generate script v1.
func createReel(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, userID := orgAndUser(c)
		if orgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "missing org context"})
		}
		var req struct {
			BriefStyle        string   `json:"brief_style"`
			Keywords          []string `json:"keywords"`
			ProductRefs       []string `json:"product_refs"`
			TargetDurationSec int      `json:"target_duration_sec"`
			MissionID         string   `json:"mission_id"`
			Source            string   `json:"source"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
		}
		res, err := deps.Service.RequestReel(c.UserContext(), orgID, reelsvc.RequestInput{
			CreatedBy: userID, MissionID: req.MissionID, Source: req.Source, BriefStyle: req.BriefStyle,
			Keywords: req.Keywords, ProductRefs: req.ProductRefs, TargetDurationSec: req.TargetDurationSec,
		})
		if err != nil {
			return c.Status(statusForErr(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(res)
	}
}

// listReels handles GET /api/reels.
func listReels(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := orgAndUser(c)
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		list, err := deps.Service.ListReels(orgID, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"reels": list, "count": len(list)})
	}
}

// getReel handles GET /api/reels/:id — reel + script + shot progress.
func getReel(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := orgAndUser(c)
		id, ok := reelID(c)
		if !ok {
			return nil
		}
		res, err := deps.Service.Get(orgID, id)
		if err != nil {
			return c.Status(statusForErr(err)).JSON(fiber.Map{"error": err.Error()})
		}
		prog, err := deps.Service.GetProgress(orgID, id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"reel": res.Reel, "script": res.Script, "shots": res.Shots,
			"status": res.Reel.Status, "shots_total": prog.ShotsTotal,
			"shots_done": prog.ShotsDone, "total_cost_usd": prog.TotalCostUSD,
		})
	}
}

// updateScript handles PATCH /api/reels/:id/script — append a new version.
func updateScript(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := orgAndUser(c)
		id, ok := reelID(c)
		if !ok {
			return nil
		}
		var req struct {
			Dialogue *string `json:"dialogue"`
			Caption  *string `json:"caption"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
		}
		res, err := deps.Service.UpdateScript(orgID, id, req.Dialogue, req.Caption)
		if err != nil {
			return c.Status(statusForErr(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(res)
	}
}

// approveReel handles POST /api/reels/:id/approve — the spend gate (idempotent).
func approveReel(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := orgAndUser(c)
		id, ok := reelID(c)
		if !ok {
			return nil
		}
		res, err := deps.Service.Approve(c.UserContext(), orgID, id)
		if err != nil {
			return c.Status(statusForErr(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"reel": res.Reel, "shots": res.Shots, "status": res.Reel.Status,
			"note": "Đang render, không thể hủy.",
		})
	}
}

// publishReel handles POST /api/reels/:id/publish — queue post_reel via the outbound spine.
func publishReel(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, userID := orgAndUser(c)
		id, ok := reelID(c)
		if !ok {
			return nil
		}
		var req struct {
			AccountID int64  `json:"account_id"`
			TargetURL string `json:"target_url"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
		}
		res, err := deps.Service.Publish(c.UserContext(), orgID, id, reelsvc.PublishInput{
			AccountID: req.AccountID, TargetURL: req.TargetURL, CreatedBy: userID,
		})
		if err != nil {
			return c.Status(statusForErr(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(res)
	}
}

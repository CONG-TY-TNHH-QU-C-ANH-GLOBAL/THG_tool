package server

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func (s *Server) getCrawlIntents(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	intents, err := s.db.ListCrawlIntentsForOrg(c.Context(), orgID, 100)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"intents": intents, "count": len(intents)})
}

func (s *Server) setCrawlIntentEnabled(c *fiber.Ctx) error {
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
	if err := s.db.SetCrawlIntentEnabled(c.Context(), orgID, id, body.Enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "crawl intent not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok", "enabled": body.Enabled})
}

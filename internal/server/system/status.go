package system

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
)

type StatusDeps struct {
	DB         *store.Store
	SessionReg *session.Registry
}

func Stats(deps StatusDeps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		stats, err := deps.DB.GetStats()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(stats)
	}
}

func SessionStats(deps StatusDeps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.SessionReg == nil {
			return c.JSON(fiber.Map{"error": "session registry not initialized"})
		}
		return c.JSON(deps.SessionReg.Stats())
	}
}

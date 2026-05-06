package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

func GeneralRateLimit() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        5000,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many requests — slow down"})
		},
	})
}

func AuthRateLimit() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        10,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "auth:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many login attempts — try again later"})
		},
	})
}

func RegistrationRateLimit() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        5,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "reg:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many registration attempts"})
		},
	})
}

func ConnectorPairingRateLimit() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        20,
		Expiration: 10 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "connector-pair:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many connector pairing attempts"})
		},
	})
}

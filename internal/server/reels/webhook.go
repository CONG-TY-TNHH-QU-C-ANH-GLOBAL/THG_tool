package reels

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/gofiber/fiber/v2"
	reelsvc "github.com/thg/scraper/internal/services/reel"
)

// renderWebhook receives one shot's render outcome from the provider. Authenticity is an
// HMAC-SHA256 over the raw body in the X-Reel-Signature header (hex). When a secret is
// configured a missing/bad signature is 401; with no secret (dev) the check is skipped,
// mirroring the Telegram webhook. The handler returns 200 on a parsed body so the provider
// does not retry a benign delivery (completion is idempotent in the service).
func renderWebhook(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		body := c.Body()
		if deps.WebhookSecret != "" && !validHMAC(body, c.Get("X-Reel-Signature"), deps.WebhookSecret) {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		var in reelsvc.RenderResult
		if err := c.BodyParser(&in); err != nil || deps.Service == nil {
			return c.SendStatus(fiber.StatusOK)
		}
		if err := deps.Service.HandleRenderResult(c.UserContext(), in); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(fiber.StatusOK)
	}
}

// validHMAC reports whether sigHex is a valid hex HMAC-SHA256(body, secret).
func validHMAC(body []byte, sigHex, secret string) bool {
	want, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(want, mac.Sum(nil))
}

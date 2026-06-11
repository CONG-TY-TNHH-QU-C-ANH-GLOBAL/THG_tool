package telegram

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/telegram/control"
)

// tgUpdate is the minimal slice of the Telegram Update we consume (a text message). Everything
// else is ignored.
type tgUpdate struct {
	Message *struct {
		Text string `json:"text"`
		From struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

// webhook validates the secret (if configured), parses the update, and hands a normalised message
// to the control service. It ALWAYS returns 200 on a parsed request so Telegram does not retry a
// benign update; only an authenticity failure returns 401. No token or update echo in responses.
func (h *Handler) webhook(c *fiber.Ctx) error {
	if h.deps.WebhookSecret != "" && c.Get("X-Telegram-Bot-Api-Secret-Token") != h.deps.WebhookSecret {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	var u tgUpdate
	if err := c.BodyParser(&u); err != nil {
		return c.SendStatus(fiber.StatusOK)
	}
	if u.Message == nil || u.Message.Text == "" {
		return c.SendStatus(fiber.StatusOK)
	}
	if h.deps.Service != nil {
		_ = h.deps.Service.HandleMessage(control.IncomingMessage{
			TgUserID:  u.Message.From.ID,
			ChatID:    u.Message.Chat.ID,
			Username:  u.Message.From.Username,
			FirstName: u.Message.From.FirstName,
			Text:      u.Message.Text,
		})
	}
	return c.SendStatus(fiber.StatusOK)
}

package telegram

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/telegram/control"
)

// tgChat is the slice of a Telegram chat we read.
type tgChat struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

// tgUpdate is the minimal slice of the Telegram Update we consume: a DM text message (commands) and
// a channel_post (the private-channel connect-code path). Everything else is ignored.
type tgUpdate struct {
	Message *struct {
		Text string `json:"text"`
		From struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Chat tgChat `json:"chat"`
	} `json:"message"`
	ChannelPost *struct {
		Text string `json:"text"`
		Chat tgChat `json:"chat"`
	} `json:"channel_post"`
}

// webhook validates the secret (if configured), parses the update, and hands it to the control
// service. It ALWAYS returns 200 on a parsed request so Telegram does not retry a benign update;
// only an authenticity failure returns 401. No token or update echo in responses.
func (h *Handler) webhook(c *fiber.Ctx) error {
	if h.deps.WebhookSecret != "" && c.Get("X-Telegram-Bot-Api-Secret-Token") != h.deps.WebhookSecret {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	var u tgUpdate
	if err := c.BodyParser(&u); err != nil || h.deps.Service == nil {
		return c.SendStatus(fiber.StatusOK)
	}
	// DM command (control-plane).
	if u.Message != nil && u.Message.Text != "" {
		_ = h.deps.Service.HandleMessage(control.IncomingMessage{
			TgUserID:  u.Message.From.ID,
			ChatID:    u.Message.Chat.ID,
			Username:  u.Message.From.Username,
			FirstName: u.Message.From.FirstName,
			Text:      u.Message.Text,
		})
	}
	// Channel post (private-channel connect via a posted code).
	if u.ChannelPost != nil && u.ChannelPost.Text != "" {
		h.deps.Service.HandleChannelPost(u.ChannelPost.Chat.ID, u.ChannelPost.Chat.Title,
			u.ChannelPost.Chat.Username, u.ChannelPost.Text)
	}
	return c.SendStatus(fiber.StatusOK)
}

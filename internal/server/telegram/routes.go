// Package telegram is the THIN webhook transport for the Telegram bot runtime. It owns ONLY:
// POST /api/telegram/webhook, the webhook-secret check, Telegram update parsing, and delegating to
// the shared control service. It contains NO binding/permission/command business logic — that all
// lives in internal/telegram/control (the single source of truth). The route is PUBLIC (Telegram
// cannot send a JWT); authenticity is established by the webhook secret, not RequireAuth.
package telegram

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/telegram/control"
)

// Deps wires the shared control service + the configured webhook secret.
type Deps struct {
	Service       *control.Service
	WebhookSecret string
}

// Handler binds the dependencies.
type Handler struct{ deps Deps }

// Routes registers the public webhook endpoint on the unauthenticated /api group.
func Routes(api fiber.Router, deps Deps) {
	h := &Handler{deps: deps}
	api.Post("/telegram/webhook", h.webhook)
}

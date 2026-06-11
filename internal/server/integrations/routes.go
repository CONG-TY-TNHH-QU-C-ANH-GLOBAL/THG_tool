// Package integrations hosts tenant-scoped, role-checked HTTP handlers for third-party
// integrations. First surface: the Telegram control-plane UI backend (status, setup/binding,
// alert preferences, audit). Every handler resolves org_id + user_id + role from the request
// context (c.Locals) and scopes all data access by org_id — no cross-tenant reads. Action
// EXECUTION is intentionally absent: this is a read-only / control-plane surface gated by the
// TELEGRAM_ACTIONS_ENABLED flag (default off).
package integrations

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	servermw "github.com/thg/scraper/internal/server/middleware"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
)

// Flags mirrors the process-level Telegram feature flags so handlers never import config. The
// router populates this from *config.Config at registration time.
type Flags struct {
	BotEnabled     bool
	NotifyEnabled  bool
	ActionsEnabled bool
	BotConfigured  bool // a bot token is present
	BotUsername    string
}

// Deps holds dependencies for the integrations handlers. Control is the SHARED Telegram domain
// service (single source of truth) — the REST handlers call it for test-notification, allow-lists,
// and audit-event names rather than re-implementing any of them.
type Deps struct {
	DB      *store.Store
	Control *control.Service
	Flags   Flags
}

// Handler binds the dependencies.
type Handler struct{ deps Deps }

// TelegramRoutes registers the Telegram integration endpoints under /settings/integrations/telegram.
// adminOnly gates org-level mutations + the full bindings/audit views; member-accessible routes
// do their own role/ownership scoping inside the handler.
func TelegramRoutes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := &Handler{deps: deps}
	g := group.Group("/settings/integrations/telegram")
	g.Get("/status", h.getStatus)            // any org member
	g.Post("/enable", adminOnly, h.enable)   // admin
	g.Post("/disable", adminOnly, h.disable) // admin

	// Per-ORG bot credential (Step 1: connect your bot). Admin-only; token save/verify are
	// rate-limited (they call Telegram getMe). The token is never returned.
	tokenLimit := servermw.AuthRateLimit()
	g.Get("/bot", adminOnly, h.getBot)
	g.Post("/bot", adminOnly, tokenLimit, h.saveBot)
	g.Post("/bot/verify", adminOnly, tokenLimit, h.verifyBot)
	g.Delete("/bot", adminOnly, h.deleteBot)

	// Notification DESTINATIONS (PRIMARY product path: Telegram channels). Admin-gated mutations.
	g.Get("/destinations", h.listDestinations)
	g.Post("/destinations", adminOnly, h.connectDestination)
	g.Delete("/destinations/:id", adminOnly, h.deleteDestination)
	g.Post("/destinations/:id/test", adminOnly, h.testDestination)
	g.Put("/destinations/:id/preferences", adminOnly, h.updateDestinationPreferences)

	// Personal DM bindings (SECONDARY: optional per-user recipients / command auth).
	g.Post("/bind-codes", h.createBindCode)          // member binds self
	g.Get("/bindings", h.listBindings)               // admin=all, member=own (in-handler)
	g.Delete("/bindings/:id", h.revokeBinding)       // admin=any, member=own (in-handler)
	g.Post("/test-notification", h.testNotification) // member, to own binding
	g.Get("/alerts", h.getAlerts)                    // any org member
	g.Put("/alerts", adminOnly, h.updateAlerts)      // admin
	g.Get("/audit", adminOnly, h.getAudit)           // admin
}

// reqCtx pulls the tenant identity from the request. orgID==0 means "no tenant context".
func reqCtx(c *fiber.Ctx) (orgID, userID int64, role string) {
	orgID, _ = c.Locals("org_id").(int64)
	userID, _ = c.Locals("user_id").(int64)
	role, _ = c.Locals("user_role").(string)
	return
}

// canViewAllBindings reports whether a role may see/manage every binding in the org (admins +
// platform owners). Everyone else is scoped to their own binding.
func canViewAllBindings(role string) bool {
	r := models.UserRole(role)
	return r == models.RoleAdmin || models.IsPlatformRole(r)
}

func noOrg(c *fiber.Ctx) error {
	return c.Status(400).JSON(fiber.Map{"error": "no org context"})
}

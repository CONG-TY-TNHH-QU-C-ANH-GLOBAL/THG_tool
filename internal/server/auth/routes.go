package auth

import (
	"github.com/gofiber/fiber/v2"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/mailer"
	"github.com/thg/scraper/internal/store"
)

// Deps holds dependencies needed by auth and onboarding handlers.
type Deps struct {
	DB                 *store.Store
	JWTSecret          string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	ChromePath         string
	ProfileDir         string
	Headless           bool
	ServerHost         string
	SSHPort            int
	Mailer             mailer.Config
}

type Handler struct {
	deps Deps
}

// Routes registers public and /api/auth/* endpoints.
func Routes(api fiber.Router, deps Deps, authLimiter, regLimiter fiber.Handler) {
	h := &Handler{deps: deps}

	authGroup := api.Group("/auth")
	authGroup.Post("/login", authLimiter, h.login)
	authGroup.Post("/signup", regLimiter, h.signupUser)
	authGroup.Post("/refresh", h.refresh)
	authGroup.Post("/logout", h.logout)

	authGroup.Get("/google", h.googleLoginRedirect)
	authGroup.Get("/google/callback", h.googleCallback)
	authGroup.Get("/google/status", h.googleStatus)
	authGroup.Post("/google/token", h.googleToken)
	authGroup.Get("/invite/:token", h.getInviteInfo)

	protected := authGroup.Group("", authpkg.RequireAuth(deps.JWTSecret))
	protected.Get("/me", h.me)
	protected.Put("/me", h.updateOwnProfile)
	protected.Put("/me/password", h.changeOwnPassword)
	protected.Get("/me/invites", h.listMyPendingInvites)
	protected.Get("/me/memberships", h.listMemberships)
	protected.Post("/refresh-membership", h.refreshMembership)
	protected.Post("/join/:token", h.acceptInvite)
}

// OnboardingRoutes registers onboarding setup.
func OnboardingRoutes(api fiber.Router, deps Deps) {
	h := &Handler{deps: deps}
	api.Post("/onboarding/setup", authpkg.RequireAuth(deps.JWTSecret), h.onboardingSetup)
}

// InviteRoutes registers org invite management under the tenant router.
func InviteRoutes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := &Handler{deps: deps}
	group.Post("/org/invites", adminOnly, h.createInvite)
	group.Get("/org/invites", adminOnly, h.listInvites)
	group.Get("/org/invites/search", adminOnly, h.searchInviteCandidates)
	group.Post("/org/invites/:id/resend", adminOnly, h.resendInvite)
	group.Delete("/org/invites/:id", adminOnly, h.revokeInvite)
}

// LoginSessionRoutes registers legacy Chrome profile login session endpoints.
func LoginSessionRoutes(group fiber.Router, deps Deps) {
	h := &Handler{deps: deps}
	group.Post("/accounts/:id/start-login", h.startLoginSession)
	group.Get("/accounts/:id/login-status", h.loginStatus)
	group.Post("/accounts/:id/capture-session", h.captureLoginSession)
	group.Post("/accounts/:id/stop-login", h.stopLoginSession)
}

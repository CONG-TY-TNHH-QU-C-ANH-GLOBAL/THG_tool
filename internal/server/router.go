package server

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	fiberws "github.com/gofiber/websocket/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thg/scraper/internal/ai"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
	serveragent "github.com/thg/scraper/internal/server/agent"
	serverauth "github.com/thg/scraper/internal/server/auth"
	"github.com/thg/scraper/internal/server/autoflow"
	"github.com/thg/scraper/internal/server/crawl"
	"github.com/thg/scraper/internal/server/leads"
	servermw "github.com/thg/scraper/internal/server/middleware"
	serverorg "github.com/thg/scraper/internal/server/org"
	serverskills "github.com/thg/scraper/internal/server/skills"
	"github.com/thg/scraper/internal/server/system"
	serverworkspace "github.com/thg/scraper/internal/server/workspace"
)

func (s *Server) registerRoutes() {
	app := s.app
	cfg := s.cfg

	// Health check — no auth, no rate limiting, for load balancers / monitors
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "ts": time.Now().Unix()})
	})

	// Prometheus metrics — no auth, scrape from internal monitoring only
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// --- Global Middleware ---

	// 1. Request logging
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} ${method} ${path} ${latency}\n",
	}))

	// 2. CORS — locked to specific origins only
	corsOrigins := "http://localhost:8080,http://127.0.0.1:8080"
	if cfg.AllowedOrigins != "" {
		corsOrigins = cfg.AllowedOrigins
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Content-Type,Authorization,X-Refresh-Token,X-Agent-Token,X-Agent-Hostname,X-Agent-OS,X-Agent-Version",
		ExposeHeaders:    "Content-Length",
		AllowCredentials: true, // needed for httpOnly cookie refresh token
	}))
	log.Printf("[Server] CORS allowed origins: %s", corsOrigins)

	// --- Route Groups ---
	api := app.Group("/api")

	// System subpackage: extension info + beta package (no auth required).
	system.Routes(api.Group("/system"), cfg.Headless)

	// 3. General rate limiting — dashboard APIs are realtime and poll session
	// state, so keep the global guard high and enforce stricter limits only on
	// auth/register endpoints.
	api.Use(servermw.GeneralRateLimit())
	// Auth routes (public) — stricter rate limit: 10 req / 15 min per IP
	authLimiter := servermw.AuthRateLimit()
	// Public org registration — stricter rate limit
	regLimiter := servermw.RegistrationRateLimit()
	orgDeps := serverorg.Deps{DB: s.db, JWTSecret: cfg.JWTSecret, Workspace: s.workspace}
	serverorg.PublicRoutes(api, orgDeps, regLimiter)

	pairingLimiter := servermw.ConnectorPairingRateLimit()
	serveragent.LocalConnectorPairingRoutes(api, serveragent.LocalConnectorDeps{DB: s.db}, pairingLimiter)
	serveragent.ConnectorRoutes(api, serveragent.Deps{
		DB:       s.db,
		AIClass:  s.aiClass,
		WSHub:    s.wsHub,
		Notifier: s.cfg.Notifier,
	})

	authDeps := serverauth.Deps{
		DB:                 s.db,
		JWTSecret:          cfg.JWTSecret,
		GoogleClientID:     cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		GoogleRedirectURI:  cfg.GoogleRedirectURI,
		ChromePath:         cfg.ChromePath,
		ProfileDir:         cfg.ProfileDir,
		Headless:           cfg.Headless,
		ServerHost:         cfg.ServerHost,
		SSHPort:            cfg.SSHPort,
		Mailer:             cfg.Mailer,
	}
	serverauth.Routes(api, authDeps, authLimiter, regLimiter)

	// Admin-only auth routes
	adminOnly := authpkg.RequireRole("admin")
	tenantReady := servermw.TenantReady()
	protectedAuth := api.Group("/auth", authpkg.RequireAuth(cfg.JWTSecret))
	serverorg.AuthAdminRoutes(protectedAuth, orgDeps, tenantReady, adminOnly)

	// Public health check (no auth required)
	systemStatusDeps := system.StatusDeps{DB: s.db, SessionReg: s.sessionReg}
	api.Get("/stats", system.Stats(systemStatusDeps))
	serverauth.OnboardingRoutes(api, authDeps)

	// Protected API routes — require JWT
	r := api.Group("", authpkg.RequireAuth(cfg.JWTSecret), tenantReady)

	leads.Routes(r, leads.Deps{
		DB:       s.db,
		JobStore: s.jobStore,
		// Lazy getter — SetUniversalClassifier wires aiClass AFTER
		// registerRoutes runs, so capturing s.aiClass directly here
		// would freeze it at nil and reclassify would always 503.
		AIClass: func() *ai.MessageGenerator { return s.aiClass },
	}, adminOnly)
	crawl.Routes(r, crawl.Deps{DB: s.db}, adminOnly)

	// Chrome Profile Login Sessions — any staff can log in their own account
	serverauth.LoginSessionRoutes(r, authDeps)
	serveragent.DashboardRoutes(r, serveragent.Deps{
		DB:       s.db,
		Agent:    s.agent,
		AIClass:  s.aiClass,
		WSHub:    s.wsHub,
		Notifier: s.cfg.Notifier,
	}, adminOnly)

	// Onboarding — new users with org_id=0 must complete this before accessing org features

	serverorg.Routes(r, orgDeps, adminOnly, authpkg.RequireRole(string(models.RoleFounder)))

	// Org invites — admin creates/lists/revokes invite links
	serverauth.InviteRoutes(r, authDeps, adminOnly)

	// Admin: manage agent tokens (JWT auth + admin role)
	adminGrp := r.Group("/admin", adminOnly)
	serveragent.AdminTokenRoutes(adminGrp, serveragent.Deps{
		DB:       s.db,
		Agent:    s.agent,
		AIClass:  s.aiClass,
		WSHub:    s.wsHub,
		Notifier: s.cfg.Notifier,
	})
	serverskills.AdminRoutes(adminGrp, serverskills.Deps{DB: s.db, Agent: s.agent})

	// Phase 6: open-prompt skill catalog. Read-only for any tenant
	// member (so the dashboard chat box can hint capabilities); enable
	// / disable requires admin role; audit feed is org-scoped.
	serverskills.Routes(r, serverskills.Deps{DB: s.db, Agent: s.agent}, adminOnly)

	// Browser workspace — per-account Chrome management
	// Chrome Extension connectors are the production path for trusted user devices.
	serveragent.LocalConnectorRoutes(r, serveragent.LocalConnectorDeps{DB: s.db}, adminOnly)

	workspaceDeps := serverworkspace.Deps{DB: s.db, Workspace: s.workspace}
	serverworkspace.Routes(r, workspaceDeps, adminOnly)

	// Self-healing Agent OS (admin only — applies patches to live files)
	if s.agentHandler != nil {
		agentGrp := r.Group("/agent", adminOnly)
		agentGrp.Post("/run", s.agentHandler.Handle)
		agentGrp.Get("/status", s.agentHandler.HandleStatus)
	}

	// Session stats (requires registry to be wired via SetSessionRegistry)
	r.Get("/sessions/stats", system.SessionStats(systemStatusDeps))

	// Analytics
	r.Get("/analytics/sentiment", system.SentimentStats(s.db))
	autoflow.Routes(r, autoflow.Deps{DB: s.db}, adminOnly)

	// WS_AUTH_ALLOW_QUERY_TOKEN gates the legacy ?token=... query
	// fallback for WS / SSE auth. Default is "1" today so legacy
	// connector / Telegram clients keep working; once telemetry shows
	// no upgrades are arriving with a query token, set it to "0" in
	// production to remove the leak surface entirely. Reading the env
	// once at boot avoids per-request env lookups.
	wsAllowQueryToken := os.Getenv("WS_AUTH_ALLOW_QUERY_TOKEN") != "0"
	if !wsAllowQueryToken {
		log.Println("[Auth] WS query-token fallback DISABLED (WS_AUTH_ALLOW_QUERY_TOKEN=0)")
	}

	// Logs SSE — Phase 4b/4c: same precedence as wsJWTAuth so the SPA,
	// programmatic clients, and the (browser) EventSource API can all
	// authenticate consistently.
	//
	//   1. access_token HttpOnly cookie
	//   2. Authorization: Bearer …      (server-to-server callers)
	//   3. ?token=… query                (legacy / EventSource fallback)
	app.Get("/api/logs/stream", func(c *fiber.Ctx) error {
		token := c.Cookies("access_token")
		if token == "" {
			if h := c.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				token = strings.TrimPrefix(h, "Bearer ")
			}
		}
		if token == "" && wsAllowQueryToken {
			token = c.Query("token")
		}
		if token == "" {
			return c.Status(401).JSON(fiber.Map{"error": "token required"})
		}
		if _, err := authpkg.ValidateAccessToken(token, cfg.JWTSecret); err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "invalid token"})
		}
		return system.StreamLogs(c)
	})

	// WebSocket auth helper — validates the JWT in this order so the
	// SPA can stop putting the access token in the URL (Phase 4b/4c):
	//
	//   1. access_token HttpOnly cookie  (set by Phase 4b login/refresh)
	//   2. Authorization: Bearer header  (server-to-server clients)
	//   3. ?token=... query param        (legacy fallback, gated by env)
	//
	// The query-param path stays so older connectors / Telegram bots that
	// haven't migrated keep working, but the SPA should rely on the
	// cookie alone — browsers send cookies on the WS upgrade request, so
	// the access token never has to land in URL access logs.
	wsJWTAuth := func(c *fiber.Ctx) error {
		if !fiberws.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		token := c.Cookies("access_token")
		if token == "" {
			if h := c.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				token = strings.TrimPrefix(h, "Bearer ")
			}
		}
		if token == "" && wsAllowQueryToken {
			token = c.Query("token")
		}
		if token == "" {
			return c.Status(401).JSON(fiber.Map{"error": "token required"})
		}
		claims, err := authpkg.ValidateAccessToken(token, cfg.JWTSecret)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "invalid token"})
		}
		c.Locals("user_id", claims.UserID)
		c.Locals("org_id", claims.OrgID)
		c.Locals("user_role", claims.Role)
		return c.Next()
	}

	// WebSocket: per-account noVNC proxy (Docker/VNC mode — primary browser view)
	app.Use("/ws/vnc/:id", wsJWTAuth)
	app.Get("/ws/vnc/:id", fiberws.New(serverworkspace.PerAccountVNCProxyHandler(workspaceDeps)))

	// WebSocket: per-account CDP screen proxy (JPEG screencast + input forwarding)
	app.Use("/ws/screen/:id", wsJWTAuth)
	app.Get("/ws/screen/:id", fiberws.New(serverworkspace.ScreenProxyHandler(workspaceDeps)))

	// WebSocket: Chrome Extension hub — token in first WS message
	app.Use("/ws/agent", func(c *fiber.Ctx) error {
		if fiberws.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/agent", fiberws.New(s.wsHub.WSHandler(s.db)))

	// The production frontend is the Next.js app on port 3000 behind nginx.
	// Keep THG AutoFlow as an API/WebSocket service only, so stale embedded UI can
	// never appear as a fallback in production.
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service":  "thg-autoflow-api",
			"status":   "ok",
			"frontend": "nextjs",
		})
	})
}

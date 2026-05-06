package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	fiberws "github.com/gofiber/websocket/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thg/scraper/internal/agentloop"
	"github.com/thg/scraper/internal/ai"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/mailer"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace"
)

// Config holds security-sensitive configuration for the API server.
type Config struct {
	Port           int
	JWTSecret      string
	AllowedOrigins string
	ChromePath     string // path to Chrome/Chromium binary for login sessions
	ProfileDir     string // base dir for Chrome persistent profiles
	Headless       bool   // true = VPS without display; Chrome login uses SSH tunnel flow
	ServerHost     string // public hostname/IP for SSH tunnel instructions
	SSHPort        int    // SSH port for tunnel (default 22)

	// Google OAuth
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	Mailer mailer.Config

	// Notifier is an optional production notification hook, normally Telegram.
	Notifier func(string)
}

func chromeExtensionStoreInfo() (string, string) {
	extensionID := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_ID"))
	storeURL := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_STORE_URL"))
	if storeURL == "" && extensionID != "" {
		storeURL = fmt.Sprintf("https://chromewebstore.google.com/detail/thg-chrome-extension/%s", extensionID)
	}
	return storeURL, extensionID
}

func chromeExtensionBetaInfo() (string, string) {
	if strings.ToLower(strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_ENABLED"))) != "true" {
		return "", ""
	}
	betaURL := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_URL"))
	if betaURL == "" {
		betaURL = "/extension-beta"
	}
	packageURL := strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_PACKAGE_URL"))
	if packageURL == "" && strings.TrimSpace(os.Getenv("CHROME_EXTENSION_BETA_PACKAGE_PATH")) != "" {
		packageURL = "/api/system/extension-beta-package"
	}
	return betaURL, packageURL
}

// Server provides the REST API and serves the Web UI.
type Server struct {
	app          *fiber.App
	db           *store.Store
	jobStore     *jobs.Store
	agent        *ai.Agent
	aiClass      *ai.MessageGenerator // optional — enables UniversalClassify for connector crawl results
	wsHub        *WSHub               // Chrome Extension WebSocket hub
	workspace    *workspace.Manager   // per-account Chrome workspace manager
	sessionReg   *session.Registry    // optional — nil disables /api/sessions/stats
	agentHandler *agentloop.Handler   // self-healing agent OS — nil = disabled
	port         int
	cfg          Config
}

// SetSessionRegistry wires in the in-memory session registry for the stats endpoint.
func (s *Server) SetSessionRegistry(r *session.Registry) {
	s.sessionReg = r
}

// SetAgentHandler wires in the self-healing agent OS handler.
// When set, POST /api/agent/run and GET /api/agent/status are enabled.
func (s *Server) SetAgentHandler(h *agentloop.Handler) {
	s.agentHandler = h
}

// SetUniversalClassifier wires the AI classifier used by the Chrome Extension
// crawl-result endpoint. When unset, that endpoint falls back to deterministic
// scoring (the pre-existing behavior).
func (s *Server) SetUniversalClassifier(mg *ai.MessageGenerator) {
	s.aiClass = mg
}

// New creates a new API server with JWT auth, RBAC, and rate limiting.
func New(db *store.Store, jobStore *jobs.Store, agent *ai.Agent, wm *workspace.Manager, cfg Config) *Server {
	if cfg.JWTSecret == "" {
		log.Println("[Server] WARNING: JWT_SECRET not set — authentication is DISABLED. Set JWT_SECRET in production!")
	}

	app := fiber.New(fiber.Config{
		AppName:                 "THG AutoFlow",
		ServerHeader:            "THG-AutoFlow",
		BodyLimit:               8 * 1024 * 1024, // local Chrome screenshots can be a few MB
		ReadTimeout:             30 * time.Second,
		WriteTimeout:            0, // no timeout — WebSocket (noVNC/agent) connections are long-lived
		IdleTimeout:             0,
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"127.0.0.1", "::1"},
	})

	s := &Server{
		app:       app,
		db:        db,
		jobStore:  jobStore,
		agent:     agent,
		port:      cfg.Port,
		cfg:       cfg,
		wsHub:     NewWSHub(),
		workspace: wm,
	}

	// Health check — no auth, no rate limiting, for load balancers / monitors
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "ts": time.Now().Unix()})
	})

	// Prometheus metrics — no auth, scrape from internal monitoring only
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// System info tells the frontend where the production Chrome Extension is installed from.
	app.Get("/api/system/info", func(c *fiber.Ctx) error {
		storeURL, extensionID := chromeExtensionStoreInfo()
		betaURL, betaPackageURL := chromeExtensionBetaInfo()
		return c.JSON(fiber.Map{
			"headless":                          cfg.Headless,
			"chrome_extension_store_url":        storeURL,
			"chrome_extension_id":               extensionID,
			"chrome_extension_beta_url":         betaURL,
			"chrome_extension_beta_package_url": betaPackageURL,
		})
	})

	// --- Global Middleware ---

	// 1. Request logging
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} ${method} ${path} ${latency}\n",
	}))

	// 2. CORS — locked to specific origins only
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

	// 3. General rate limiting — dashboard APIs are realtime and poll session
	// state, so keep the global guard high and enforce stricter limits only on
	// auth/register endpoints.
	api.Use(limiter.New(limiter.Config{
		Max:        5000,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many requests — slow down"})
		},
	}))

	// Auth routes (public) — stricter rate limit: 10 req / 15 min per IP
	authLimiter := limiter.New(limiter.Config{
		Max:        10,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "auth:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many login attempts — try again later"})
		},
	})
	// Public org registration — stricter rate limit
	regLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "reg:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many registration attempts"})
		},
	})
	api.Post("/register", regLimiter, s.registerOrg)

	pairingLimiter := limiter.New(limiter.Config{
		Max:        20,
		Expiration: 10 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "connector-pair:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many connector pairing attempts"})
		},
	})
	api.Post("/connectors/pair", pairingLimiter, s.claimLocalConnectorPairingCode)
	// Connector-auth aliases keep the extension control plane under the same
	// /api/connectors namespace as pairing. /api/agent remains for older agents.
	api.Post("/connectors/heartbeat", s.agentAuth, s.agentHeartbeat)
	api.Post("/connectors/chrome-status", s.agentAuth, s.agentChromeStatus)
	api.Get("/connectors/browser-targets", s.agentAuth, s.agentBrowserTargets)
	api.Post("/connectors/screenshot", s.agentAuth, s.agentScreenshot)
	api.Post("/connectors/crawl-result", s.agentAuth, s.agentConnectorCrawlResult)
	api.Post("/connectors/crawl-progress", s.agentAuth, s.agentConnectorCrawlProgress)
	api.Get("/connectors/commands", s.agentAuth, s.agentConnectorCommands)
	api.Post("/connectors/commands/:id/done", s.agentAuth, s.agentConnectorCommandDone)
	api.Get("/connectors/outbox", s.agentAuth, s.agentGetOutbox)
	api.Post("/connectors/outbox/:id/sent", s.agentAuth, s.agentOutboxSent)
	api.Post("/connectors/outbox/:id/failed", s.agentAuth, s.agentOutboxFailed)

	// Agent API — authenticated with X-Agent-Token header (no JWT needed).
	// Keep this before the catch-all protected API group below; otherwise
	// /api/agent/* can be intercepted by JWT auth and Chrome Extensions see
	// {"error":"authentication required"} even with a valid device token.
	agentGrp := api.Group("/agent", s.agentAuth)
	agentGrp.Post("/heartbeat", s.agentHeartbeat)
	agentGrp.Post("/chrome-status", s.agentChromeStatus)
	agentGrp.Get("/browser-targets", s.agentBrowserTargets)
	agentGrp.Post("/screenshot", s.agentScreenshot)
	agentGrp.Post("/crawl-result", s.agentConnectorCrawlResult)
	agentGrp.Post("/crawl-progress", s.agentConnectorCrawlProgress)
	agentGrp.Get("/commands", s.agentConnectorCommands)
	agentGrp.Post("/commands/:id/done", s.agentConnectorCommandDone)
	agentGrp.Get("/outbox", s.agentGetOutbox)
	agentGrp.Post("/outbox/:id/sent", s.agentOutboxSent)
	agentGrp.Post("/outbox/:id/failed", s.agentOutboxFailed)
	agentGrp.Get("/images", s.agentServeImage)

	authGroup := api.Group("/auth")
	authGroup.Post("/login", authLimiter, s.login)
	authGroup.Post("/signup", regLimiter, s.signupUser)
	authGroup.Post("/refresh", s.refresh)
	authGroup.Post("/logout", s.logout) // no JWT required — only needs refresh token cookie

	// Google OAuth
	authGroup.Get("/google", s.googleLoginRedirect)
	authGroup.Get("/google/callback", s.googleCallback)
	authGroup.Get("/google/status", s.googleStatus)
	authGroup.Post("/google/token", s.googleToken)

	// Invite info (public — shows org name + email hint for join page)
	authGroup.Get("/invite/:token", s.getInviteInfo)

	// Auth routes (require valid JWT)
	protected := authGroup.Group("", authpkg.RequireAuth(cfg.JWTSecret))
	protected.Get("/me", s.me)
	protected.Put("/me", s.updateOwnProfile)
	protected.Put("/me/password", s.changeOwnPassword)
	protected.Post("/join/:token", s.acceptInvite)

	// Admin-only auth routes
	adminOnly := authpkg.RequireRole("admin")
	tenantReady := func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		role, _ := c.Locals("user_role").(string)
		if orgID == 0 && !models.IsPlatformRole(models.UserRole(role)) {
			return c.Status(403).JSON(fiber.Map{
				"error": "onboarding required",
				"code":  "ONBOARDING_REQUIRED",
			})
		}
		if orgID != 0 && models.IsPlatformRole(models.UserRole(role)) {
			return c.Status(403).JSON(fiber.Map{
				"error": "invalid platform role context",
			})
		}
		return c.Next()
	}
	protected.Post("/users", tenantReady, adminOnly, s.createOrgUser)
	protected.Get("/users", tenantReady, adminOnly, s.listUsers)
	protected.Put("/users/:id", tenantReady, adminOnly, s.adminUpdateUser)
	protected.Delete("/users/:id", tenantReady, adminOnly, s.adminDeleteUser)
	protected.Get("/audit", tenantReady, adminOnly, s.getAuditLogs)

	// Public health check (no auth required)
	api.Get("/stats", s.getStats)
	api.Get("/public/org-assets/:orgID/:kind", s.serveOrgAsset)
	api.Post("/onboarding/setup", authpkg.RequireAuth(cfg.JWTSecret), s.onboardingSetup)

	// Protected API routes — require JWT
	r := api.Group("", authpkg.RequireAuth(cfg.JWTSecret), tenantReady)

	// Leads — sales can read and delete individual; admin deletes all
	r.Get("/leads", s.getLeads)
	r.Delete("/leads/all", adminOnly, s.deleteAllLeads)
	r.Delete("/leads/:id", s.deleteLead)

	// Niches — admin only (system config)
	r.Get("/niches", s.getNiches)
	r.Post("/niches", adminOnly, s.addNiche)
	r.Delete("/niches/:slug", adminOnly, s.deleteNiche)

	// Posts — sales can view; admin deletes
	r.Get("/posts", s.getPosts)
	r.Delete("/posts/all", adminOnly, s.deleteAllPosts)
	r.Delete("/posts/:id", adminOnly, s.deletePost)

	// Jobs — sales can view and create; admin cancels
	r.Get("/jobs", s.getJobs)
	r.Post("/jobs", s.createJob)
	r.Delete("/jobs/:id", adminOnly, s.cancelJob)
	r.Get("/crawl-intents", s.getCrawlIntents)
	r.Put("/crawl-intents/:id/enabled", adminOnly, s.setCrawlIntentEnabled)

	// Groups — sales can view; admin manages
	r.Get("/groups", s.getGroups)
	r.Post("/groups", adminOnly, s.addGroup)
	r.Put("/groups/:id/toggle", adminOnly, s.toggleGroup)
	r.Delete("/groups/:id", adminOnly, s.deleteGroup)

	// Accounts — all staff can add their own; admin manages all
	r.Get("/accounts", s.getAccounts)
	r.Post("/accounts", s.addAccount) // any staff can register their FB account
	r.Get("/system/extension-beta-package", s.serveExtensionBetaPackage)
	r.Put("/accounts/:id/status", adminOnly, s.updateAccountStatus)
	r.Put("/accounts/:id/cookies", adminOnly, s.updateAccountCookies)
	r.Delete("/accounts/:id", adminOnly, s.deleteAccount)

	// Chrome Profile Login Sessions — any staff can log in their own account
	r.Post("/accounts/:id/start-login", s.startLoginSession)
	r.Get("/accounts/:id/login-status", s.loginStatus)
	r.Post("/accounts/:id/capture-session", s.captureLoginSession)
	r.Post("/accounts/:id/stop-login", s.stopLoginSession)

	// AI Agent — all authenticated users
	r.Post("/ai/prompt", s.aiPrompt)
	r.Get("/ai/history", s.aiHistory)
	r.Delete("/ai/history", s.aiDeleteHistory)
	r.Delete("/ai/history/:id", s.aiDeleteHistoryItem)

	// Outbound messages — sales handles approve/reject; admin bulk-deletes
	r.Get("/outbox", s.getOutbox)
	r.Post("/outbox/draft", s.draftOutbound)
	r.Delete("/outbox/comments/all", adminOnly, s.deleteAllOutboundComments)
	r.Put("/outbox/:id/approve", s.approveOutbound)
	r.Put("/outbox/:id/reject", s.rejectOutbound)
	r.Put("/outbox/:id/content", s.editOutbound)
	r.Delete("/outbox/:id", s.deleteOutbound)

	// Onboarding — new users with org_id=0 must complete this before accessing org features

	// Org self-service (any authenticated user sees their org)
	r.Get("/org", s.getMyOrg)
	r.Put("/org", authpkg.RequireRole("admin"), s.updateOrg)
	r.Post("/org/assets/:kind", adminOnly, s.uploadOrgAsset)

	// Org invites — admin creates/lists/revokes invite links
	r.Post("/org/invites", adminOnly, s.createInvite)
	r.Get("/org/invites", adminOnly, s.listInvites)
	r.Post("/org/invites/:id/resend", adminOnly, s.resendInvite)
	r.Delete("/org/invites/:id", adminOnly, s.revokeInvite)

	// Superadmin: org management — /superadmin prefix keeps it separate from /admin
	superAdminGrp := r.Group("/superadmin", authpkg.RequireRole(string(models.RoleFounder)))
	superAdminGrp.Get("/orgs", s.listOrgs)
	superAdminGrp.Put("/orgs/:id", s.adminUpdateOrg)
	superAdminGrp.Delete("/orgs/:id", s.superAdminDeleteOrg)
	superAdminGrp.Get("/accounts", s.superAdminAccounts)
	superAdminGrp.Delete("/accounts/:id", s.superAdminDeleteAccount)
	superAdminGrp.Get("/users", s.superAdminUsers)
	superAdminGrp.Delete("/users/:id", s.superAdminDeleteUser)
	superAdminGrp.Get("/sessions", s.superAdminSessions)
	superAdminGrp.Delete("/sessions/:id", s.superAdminTerminateSession)
	superAdminGrp.Post("/query", s.superAdminQuery)

	// Admin: manage agent tokens (JWT auth + admin role)
	adminGrp := r.Group("/admin", adminOnly)
	adminGrp.Post("/agent-tokens", s.agentCreateToken)
	adminGrp.Get("/agent-tokens", s.agentListTokens)
	adminGrp.Delete("/agent-tokens/:id", s.agentRevokeToken)
	adminGrp.Get("/skills", s.skillsAll)

	// Phase 6: open-prompt skill catalog. Read-only for any tenant
	// member (so the dashboard chat box can hint capabilities); enable
	// / disable requires admin role; audit feed is org-scoped.
	r.Get("/skills", s.skillsList)
	r.Get("/skills/executions", s.skillExecutions)
	r.Put("/skills/:id/enable", adminOnly, s.skillEnable)
	r.Put("/skills/:id/disable", adminOnly, s.skillDisable)

	// Browser workspace — per-account Chrome management
	// Chrome Extension connectors are the production path for trusted user devices.
	r.Get("/connectors", s.listLocalConnectors)
	r.Get("/connectors/screen", s.getLocalConnectorScreen)
	r.Post("/connectors/input", s.createConnectorInputCommand)
	r.Post("/connectors", s.createLocalConnectorPairingCode) // legacy alias: returns a short-lived pairing code
	r.Post("/connectors/pairing-code", s.createLocalConnectorPairingCode)
	r.Post("/connectors/:id/disconnect", s.disconnectLocalConnectorPost)
	r.Put("/connectors/:id/account", adminOnly, s.assignLocalConnectorAccount)
	r.Delete("/connectors/:id", s.revokeLocalConnector)

	r.Get("/browser/workspaces", s.workspaceList)
	r.Post("/browser/workspaces/new", s.workspaceNew) // must be before /:id routes
	r.Post("/browser/workspaces/:id/start", s.workspaceStart)
	r.Post("/browser/workspaces/:id/stop", s.workspaceStop)
	r.Post("/browser/workspaces/:id/navigate", s.workspaceNavigate)
	r.Post("/browser/workspaces/:id/sync-session", s.workspaceSyncSession)
	r.Post("/browser/workspaces/:id/set-logged-in", s.workspaceSetLoggedIn)
	r.Post("/browser/workspaces/:id/resolve-checkpoint", s.resolveCheckpoint)
	r.Get("/browser/checkpoints", s.listCheckpoints)

	// Self-healing Agent OS (admin only — applies patches to live files)
	if s.agentHandler != nil {
		agentGrp := r.Group("/agent", adminOnly)
		agentGrp.Post("/run", s.agentHandler.Handle)
		agentGrp.Get("/status", s.agentHandler.HandleStatus)
	}

	// Session stats (requires registry to be wired via SetSessionRegistry)
	r.Get("/sessions/stats", s.getSessionStats)

	// Analytics
	r.Get("/analytics/sentiment", s.getSentimentStats)
	r.Get("/billing/summary", s.billingSummary)
	r.Get("/context/business", s.getBusinessContext)
	r.Put("/context/business", s.updateBusinessContext)

	// AutoFlow: Staff KPI
	r.Get("/staff", s.autoflowGetStaff)
	r.Put("/staff/:id/kpi", adminOnly, s.autoflowUpdateKPI)

	// AutoFlow: KPI Config (admin sets weights)
	r.Get("/kpi/config", s.autoflowGetKPIConfig)
	r.Put("/kpi/config", adminOnly, s.autoflowUpdateKPIConfig)

	// AutoFlow: Private Files
	r.Get("/files", s.autoflowListFiles)
	r.Post("/files", s.autoflowUploadFile)
	r.Delete("/files/:id", s.autoflowDeleteFile)
	r.Get("/data-sources", s.listDataSources)
	r.Post("/data-sources", adminOnly, s.createDataSource)
	r.Post("/data-sources/:id/sync", adminOnly, s.syncDataSource)
	r.Delete("/data-sources/:id", adminOnly, s.deleteDataSource)

	// AutoFlow: Conversation Threads
	r.Get("/threads", s.autoflowListThreads)
	r.Get("/threads/:id/messages", s.autoflowGetMessages)
	r.Post("/threads/:id/messages", s.autoflowSendMessage)

	// AutoFlow: Facebook Session summary
	r.Get("/facebook/status", s.autoflowFacebookStatus)

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

	// Logs SSE — Phase 4b/4c: same precedence as wsJWTAuth so the SPA,
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
		return s.streamLogs(c)
	})

	// WebSocket auth helper — validates the JWT in this order so the
	// SPA can stop putting the access token in the URL (Phase 4b/4c):
	//
	//   1. access_token HttpOnly cookie  (set by Phase 4b login/refresh)
	//   2. Authorization: Bearer header  (server-to-server clients)
	//   3. ?token=... query param        (legacy fallback, gated by env)
	//
	// The query-param path stays so older connectors / Telegram bots that
	// haven't migrated keep working, but the SPA should rely on the
	// cookie alone — browsers send cookies on the WS upgrade request, so
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

	// WebSocket: per-account noVNC proxy (Docker/VNC mode — primary browser view)
	app.Use("/ws/vnc/:id", wsJWTAuth)
	app.Get("/ws/vnc/:id", fiberws.New(s.perAccountVNCProxyHandler()))

	// WebSocket: per-account CDP screen proxy (JPEG screencast + input forwarding)
	app.Use("/ws/screen/:id", wsJWTAuth)
	app.Get("/ws/screen/:id", fiberws.New(s.screenProxyHandler()))

	// WebSocket: Chrome Extension hub — token in first WS message
	app.Use("/ws/agent", func(c *fiber.Ctx) error {
		if fiberws.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/agent", fiberws.New(s.wsHub.wsHandler(db)))

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

	return s
}

// Start begins serving the API.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("[Server] Starting on %s (JWT auth: %v)", addr, s.cfg.JWTSecret != "")
	return s.app.Listen(addr)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}

// --- Handlers ---

func (s *Server) getSessionStats(c *fiber.Ctx) error {
	if s.sessionReg == nil {
		return c.JSON(fiber.Map{"error": "session registry not initialized"})
	}
	return c.JSON(s.sessionReg.Stats())
}

func (s *Server) getStats(c *fiber.Ctx) error {
	stats, err := s.db.GetStats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}

func (s *Server) getLeads(c *fiber.Ctx) error {
	score := c.Query("score", "")
	niche := c.Query("niche", "")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	orgID, _ := c.Locals("org_id").(int64)

	var (
		leads []models.Lead
		err   error
	)
	// The Chrome Extension crawl path stores into task_leads first, then best-effort
	// mirrors into legacy leads. Merge both tables for the main dashboard view so
	// production users immediately see extension-crawled results even when the
	// legacy mirror is empty or delayed.
	if niche == "" && offset == 0 {
		leads, err = s.db.GetAutomationLeadsForOrg(orgID, score, limit)
	} else {
		leads, err = s.db.GetLeadsFiltered(score, niche, limit, offset, orgID)
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"leads": leads, "count": len(leads)})
}

func (s *Server) getNiches(c *fiber.Ctx) error {
	niches, err := s.db.GetNiches()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"niches": niches, "count": len(niches)})
}

func (s *Server) addNiche(c *fiber.Ctx) error {
	var req struct {
		Slug  string `json:"slug"`
		Name  string `json:"name"`
		Emoji string `json:"emoji"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Slug == "" || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "slug and name required"})
	}
	n := &models.Niche{Slug: req.Slug, Name: req.Name, Emoji: req.Emoji}
	id, err := s.db.InsertNiche(n)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"id": id, "slug": req.Slug})
}

func (s *Server) deleteNiche(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "logistics" {
		return c.Status(400).JSON(fiber.Map{"error": "cannot delete default niche"})
	}
	if err := s.db.DeleteNiche(slug); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (s *Server) deleteLead(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.DeleteLead(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (s *Server) getPosts(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	orgID, _ := c.Locals("org_id").(int64)

	posts, err := s.db.GetRecentPosts(limit, offset, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"posts": posts, "count": len(posts)})
}

func (s *Server) deletePost(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.DeletePost(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (s *Server) deleteAllPosts(c *fiber.Ctx) error {
	count, err := s.db.DeleteAllPosts()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[API] Deleted all posts: %d removed", count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count})
}

func (s *Server) deleteAllLeads(c *fiber.Ctx) error {
	niche := c.Query("niche", "")
	count, err := s.db.DeleteLeads(niche)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	scope := "all"
	if niche != "" {
		scope = niche
	}
	userID, _ := c.Locals("user_id").(int64)
	s.db.InsertAuditLog(userID, "delete_leads", c.IP(), fmt.Sprintf(`{"scope":%q,"count":%d}`, scope, count))
	log.Printf("[API] Deleted leads (scope=%s): %d removed", scope, count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count, "scope": scope})
}

func (s *Server) getJobs(c *fiber.Ctx) error {
	status := c.Query("status", "")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	list, err := s.jobStore.List(c.Context(), status, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"jobs": list, "count": len(list)})
}

func (s *Server) createJob(c *fiber.Ctx) error {
	var req struct {
		Intent    string `json:"intent"`
		Platform  string `json:"platform"`
		Target    string `json:"target"`
		AccountID int64  `json:"account_id"`
		Text      string `json:"text"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Intent == "" {
		req.Intent = "facebook_crawl"
	}
	if req.Target == "" {
		return c.Status(400).JSON(fiber.Map{"error": "target URL is required for crawler jobs; use /api/ai/prompt for free-form agent prompts"})
	}
	if req.Platform == "" {
		req.Platform = "facebook"
	}

	sourceType := req.Platform + "_group"
	if req.Platform == "web" || req.Platform == "website" {
		sourceType = "web_url"
	}
	task := &jobs.Task{
		SchemaVersion: "1",
		TaskID:        fmt.Sprintf("api-%s-%d", req.Intent, time.Now().UnixMilli()),
		AccountID:     req.AccountID,
		Intent:        req.Intent,
		Keywords:      strings.Fields(req.Text),
		CrawlPlan: jobs.CrawlPlan{
			Sources:  []jobs.Source{{Type: sourceType, URL: req.Target}},
			MaxItems: 50,
		},
		OutputSchema:        "open_crawler_v1",
		OutputSchemaVersion: "1",
	}
	task.OrgID, _ = c.Locals("org_id").(int64)
	payload, _ := json.Marshal(task)
	j, err := s.jobStore.Submit(c.Context(), task, string(payload))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"job_id": j.ID, "task_id": j.TaskID, "status": "submitted"})
}

func (s *Server) cancelJob(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid job id"})
	}

	if err := s.jobStore.Cancel(c.Context(), id); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "canceled"})
}

func (s *Server) getGroups(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	groups, err := s.db.GetAllGroups(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"groups": groups, "count": len(groups)})
}

func (s *Server) addGroup(c *fiber.Ctx) error {
	var req struct {
		Platform string `json:"platform"`
		Name     string `json:"name"`
		URL      string `json:"url"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	groupOrgID, _ := c.Locals("org_id").(int64)
	group := &models.Group{
		OrgID:     groupOrgID,
		Platform:  models.Platform(req.Platform),
		Name:      req.Name,
		URL:       req.URL,
		Active:    true,
		JoinState: "none",
	}

	id, err := s.db.AddGroup(group)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"group_id": id})
}

func (s *Server) toggleGroup(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var req struct {
		Active bool `json:"active"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := s.db.ToggleGroup(id, req.Active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

func (s *Server) deleteGroup(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := s.db.DeleteGroup(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

// --- v2: Account Handlers ---

func (s *Server) getAccounts(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	accounts, err := s.db.GetAllAccounts(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	for i := range accounts {
		accounts[i].CookiesJSON = "[REDACTED]"
	}
	return c.JSON(fiber.Map{"accounts": accounts, "count": len(accounts)})
}

func (s *Server) addAccount(c *fiber.Ctx) error {
	var req struct {
		Platform    string `json:"platform"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		CookiesJSON string `json:"cookies_json"` // optional — staff uses Chrome login instead
		ProxyURL    string `json:"proxy_url"`
		Notes       string `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	if req.Platform == "" {
		req.Platform = "facebook"
	}

	userID, _ := c.Locals("user_id").(int64)
	orgID, _ := c.Locals("org_id").(int64)

	acc := &models.Account{
		OrgID:          orgID,
		Platform:       models.Platform(req.Platform),
		Name:           req.Name,
		Email:          req.Email,
		CookiesJSON:    req.CookiesJSON,
		ProxyURL:       req.ProxyURL,
		Status:         models.AccountInactive,
		Notes:          req.Notes,
		AssignedUserID: userID,
	}
	// If cookies provided upfront, mark active immediately
	if req.CookiesJSON != "" {
		acc.Status = models.AccountActive
	}

	id, err := s.db.AddAccount(acc)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	log.Printf("[Accounts] Account created: %s (id=%d) by user %d", req.Name, id, userID)
	return c.Status(201).JSON(fiber.Map{"account_id": id})
}

func (s *Server) updateAccountStatus(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	orgID, _ := c.Locals("org_id").(int64)
	acc, err := s.db.GetAccountForOrg(id, orgID)
	if err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}

	if err := s.db.UpdateAccountStatus(id, models.AccountStatus(req.Status)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

func (s *Server) updateAccountCookies(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var req struct {
		CookiesJSON string `json:"cookies_json"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	orgID, _ := c.Locals("org_id").(int64)
	if acc, err := s.db.GetAccountForOrg(id, orgID); err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}

	if err := s.db.UpdateAccountCookies(id, req.CookiesJSON); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "updated"})
}

func (s *Server) deleteAccount(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	orgID, _ := c.Locals("org_id").(int64)
	if acc, err := s.db.GetAccountForOrg(id, orgID); err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}

	if err := s.db.DeleteAccount(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

// --- v2: AI Agent Handlers ---

func (s *Server) aiPrompt(c *fiber.Ctx) error {
	var req struct {
		Prompt    string `json:"prompt"`
		AccountID int64  `json:"account_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if s.agent == nil || !s.agent.Available() {
		return c.Status(503).JSON(fiber.Map{"error": "AI agent not configured (check OPENAI_API_KEY)"})
	}

	prompt := strings.TrimSpace(req.Prompt)
	orgID, _ := c.Locals("org_id").(int64)
	response, err := s.agent.ProcessPromptForOrgWithAccount(c.Context(), prompt, "dashboard", orgID, req.AccountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"response": response})
}

func (s *Server) aiHistory(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	orgID, _ := c.Locals("org_id").(int64)
	history, err := s.db.GetPromptHistoryForOrg(orgID, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"history": history, "count": len(history)})
}

func (s *Server) aiDeleteHistoryItem(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid history id"})
	}
	if err := s.db.DeletePromptLogForOrg(orgID, id); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(404).JSON(fiber.Map{"error": "history item not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "deleted_id": id})
}

func (s *Server) aiDeleteHistory(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	deleted, err := s.db.DeleteAllPromptLogsForOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "deleted": deleted})
}

// --- v3: Outbound Message Handlers ---

func (s *Server) getOutbox(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	status := c.Query("status", "")
	msgType := c.Query("type", "")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	messages, err := s.db.GetOutboundByFilterForOrg(orgID, status, msgType, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	counts, _ := s.db.CountOutboundByStatusForOrg(orgID)
	return c.JSON(fiber.Map{"messages": messages, "count": len(messages), "counts": counts})
}

func (s *Server) draftOutbound(c *fiber.Ctx) error {
	var req struct {
		Type       string `json:"type"` // comment, inbox
		AccountID  int64  `json:"account_id"`
		TargetURL  string `json:"target_url"`
		TargetName string `json:"target_name"`
		Content    string `json:"content"` // manual content (optional, AI generates if empty)
		Context    string `json:"context"` // original post for AI context
		Auto       bool   `json:"auto"`    // true = queue as approved for immediate agent execution
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Type == "" {
		req.Type = "comment"
	}
	if req.Type != "comment" && req.Type != "inbox" && req.Type != "group_post" && req.Type != "profile_post" {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported outbound type"})
	}
	orgID := c.Locals("org_id").(int64)
	if req.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "account_id is required"})
	}
	if acct, err := s.db.GetAccountForOrg(req.AccountID, orgID); err != nil || acct == nil {
		return c.Status(403).JSON(fiber.Map{"error": "account does not belong to this organization"})
	}

	guard, err := s.db.CanQueueOutboundForOrg(orgID, req.Type, req.TargetURL, req.TargetURL, 24*time.Hour)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !guard.Allowed {
		return c.Status(409).JSON(fiber.Map{
			"error":       "outbound_blocked",
			"reason":      guard.Reason,
			"existing_id": guard.ExistingID,
		})
	}

	status := models.OutboundDraft
	if req.Auto {
		status = models.OutboundApproved
	}
	msg := &models.OutboundMessage{
		OrgID:      orgID,
		Type:       req.Type,
		Platform:   models.PlatformFacebook,
		AccountID:  req.AccountID,
		TargetURL:  req.TargetURL,
		TargetName: req.TargetName,
		Content:    req.Content,
		Context:    req.Context,
		Status:     status,
	}

	id, err := s.db.InsertOutboundMessage(msg)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if status == models.OutboundApproved && s.wsHub != nil {
		s.wsHub.NotifyOutboxReady(1)
	}
	s.notifyOutboundQueued(orgID, req.AccountID, id, req.Type, status)
	return c.Status(201).JSON(fiber.Map{"message_id": id, "status": status})
}

func (s *Server) approveOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundApproved); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	if s.wsHub != nil {
		s.wsHub.NotifyOutboxReady(1)
	}
	s.notifyOutboundStatus(orgID, id, models.OutboundApproved)
	return c.JSON(fiber.Map{"status": "approved", "message": "Đã duyệt! Tin nhắn sẽ được gửi tự động."})
}

func (s *Server) rejectOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundRejected); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	return c.JSON(fiber.Map{"status": "rejected"})
}

func (s *Server) editOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := s.db.UpdateOutboundContentForOrg(orgID, id, req.Content); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

func (s *Server) deleteOutbound(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.DeleteOutboundForOrg(orgID, id); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (s *Server) deleteAllOutboundComments(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	count, err := s.db.DeleteAllOutboundCommentsForOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[API] Reset all outbound comments: %d deleted", count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count})
}

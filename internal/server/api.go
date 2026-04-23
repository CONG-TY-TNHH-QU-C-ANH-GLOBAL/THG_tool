package server

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/thg/scraper/internal/ai"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/queue"
	"github.com/thg/scraper/internal/store"
)

//go:embed all:static
var staticFS embed.FS

// Config holds security-sensitive configuration for the API server.
type Config struct {
	Port           int
	JWTSecret      string
	AllowedOrigins string
}

// Server provides the REST API and serves the Web UI.
type Server struct {
	app   *fiber.App
	db    *store.Store
	queue *queue.Queue
	agent *ai.Agent
	port  int
	cfg   Config
}

// New creates a new API server with JWT auth, RBAC, and rate limiting.
func New(db *store.Store, q *queue.Queue, agent *ai.Agent, cfg Config) *Server {
	if cfg.JWTSecret == "" {
		log.Println("[Server] WARNING: JWT_SECRET not set — authentication is DISABLED. Set JWT_SECRET in production!")
	}

	app := fiber.New(fiber.Config{
		AppName:      "THG Agentic Scraper",
		ServerHeader: "THG-Scraper",
		BodyLimit:    4 * 1024 * 1024, // 4 MB max body
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	})

	s := &Server{
		app:   app,
		db:    db,
		queue: q,
		agent: agent,
		port:  cfg.Port,
		cfg:   cfg,
	}

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
		AllowMethods:     "GET,POST,PUT,DELETE",
		AllowHeaders:     "Content-Type,Authorization",
		ExposeHeaders:    "Content-Length",
		AllowCredentials: true, // needed for httpOnly cookie refresh token
	}))
	log.Printf("[Server] CORS allowed origins: %s", corsOrigins)

	// 3. General rate limiting — 100 req / 15 min per IP
	app.Use(limiter.New(limiter.Config{
		Max:        100,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "too many requests — slow down"})
		},
	}))

	// --- Route Groups ---
	api := app.Group("/api")

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
	authGroup := api.Group("/auth")
	authGroup.Post("/login", authLimiter, s.login)
	authGroup.Post("/refresh", authLimiter, s.refresh)

	// Auth routes (require valid JWT)
	protected := authGroup.Group("", authpkg.RequireAuth(cfg.JWTSecret))
	protected.Post("/logout", s.logout)
	protected.Get("/me", s.me)
	protected.Put("/me", s.updateOwnProfile)
	protected.Put("/me/password", s.changeOwnPassword)

	// Admin-only auth routes
	adminOnly := authpkg.RequireRole("admin")
	protected.Post("/users", adminOnly, s.createUser)
	protected.Get("/users", adminOnly, s.listUsers)
	protected.Put("/users/:id", adminOnly, s.adminUpdateUser)
	protected.Delete("/users/:id", adminOnly, s.adminDeleteUser)
	protected.Get("/audit", adminOnly, s.getAuditLogs)

	// Public health check (no auth required)
	api.Get("/stats", s.getStats)

	// Protected API routes — require JWT
	r := api.Group("", authpkg.RequireAuth(cfg.JWTSecret))

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

	// Groups — sales can view; admin manages
	r.Get("/groups", s.getGroups)
	r.Post("/groups", adminOnly, s.addGroup)
	r.Put("/groups/:id/toggle", adminOnly, s.toggleGroup)
	r.Delete("/groups/:id", adminOnly, s.deleteGroup)

	// Accounts — sales can view (cookies redacted); admin manages
	r.Get("/accounts", s.getAccounts)
	r.Post("/accounts", adminOnly, s.addAccount)
	r.Put("/accounts/:id/status", adminOnly, s.updateAccountStatus)
	r.Put("/accounts/:id/cookies", adminOnly, s.updateAccountCookies)
	r.Delete("/accounts/:id", adminOnly, s.deleteAccount)

	// AI Agent — all authenticated users
	r.Post("/ai/prompt", s.aiPrompt)
	r.Get("/ai/history", s.aiHistory)

	// Outbound messages — sales handles approve/reject; admin bulk-deletes
	r.Get("/outbox", s.getOutbox)
	r.Post("/outbox/draft", s.draftOutbound)
	r.Delete("/outbox/comments/all", adminOnly, s.deleteAllOutboundComments)
	r.Put("/outbox/:id/approve", s.approveOutbound)
	r.Put("/outbox/:id/reject", s.rejectOutbound)
	r.Put("/outbox/:id/content", s.editOutbound)
	r.Delete("/outbox/:id", s.deleteOutbound)

	// Serve embedded static files (Web UI)
	staticSub, _ := fs.Sub(staticFS, "static")
	app.Use("/", filesystem.New(filesystem.Config{
		Root:         http.FS(staticSub),
		Browse:       false,
		Index:        "index.html",
		NotFoundFile: "index.html",
	}))

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

	leads, err := s.db.GetLeadsFiltered(score, niche, limit, offset)
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

	posts, err := s.db.GetRecentPosts(limit, offset)
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

	jobs, err := s.db.GetJobs(status, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"jobs": jobs, "count": len(jobs)})
}

func (s *Server) createJob(c *fiber.Ctx) error {
	var req struct {
		Type     string `json:"type"`
		Platform string `json:"platform"`
		Target   string `json:"target"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	job := models.Job{
		Type:     models.JobType(req.Type),
		Platform: models.Platform(req.Platform),
		Target:   req.Target,
	}

	jobID, err := s.queue.Submit(job)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"job_id": jobID, "status": "submitted"})
}

func (s *Server) cancelJob(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid job id"})
	}

	if err := s.queue.Cancel(id); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "canceled"})
}

func (s *Server) getGroups(c *fiber.Ctx) error {
	groups, err := s.db.GetAllGroups()
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

	group := &models.Group{
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
	accounts, err := s.db.GetAllAccounts()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// Redact sensitive session cookies from API responses
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
		CookiesJSON string `json:"cookies_json"`
		ProxyURL    string `json:"proxy_url"`
		Notes       string `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Platform == "" {
		req.Platform = "facebook"
	}

	acc := &models.Account{
		Platform:    models.Platform(req.Platform),
		Name:        req.Name,
		Email:       req.Email,
		CookiesJSON: req.CookiesJSON,
		ProxyURL:    req.ProxyURL,
		Status:      models.AccountActive,
		Notes:       req.Notes,
	}

	id, err := s.db.AddAccount(acc)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

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

	if err := s.db.DeleteAccount(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

// --- v2: AI Agent Handlers ---

func (s *Server) aiPrompt(c *fiber.Ctx) error {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if s.agent == nil || !s.agent.Available() {
		return c.Status(503).JSON(fiber.Map{"error": "AI agent not configured (check OPENAI_API_KEY)"})
	}

	response, err := s.agent.ProcessPrompt(c.Context(), req.Prompt, "dashboard")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"response": response})
}

func (s *Server) aiHistory(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	history, err := s.db.GetPromptHistory(limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"history": history, "count": len(history)})
}

// --- v3: Outbound Message Handlers ---

func (s *Server) getOutbox(c *fiber.Ctx) error {
	status := c.Query("status", "")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	messages, err := s.db.GetOutboundByStatus(status, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	counts, _ := s.db.CountOutboundByStatus()
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
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Type == "" {
		req.Type = "comment"
	}

	msg := &models.OutboundMessage{
		Type:       req.Type,
		Platform:   models.PlatformFacebook,
		AccountID:  req.AccountID,
		TargetURL:  req.TargetURL,
		TargetName: req.TargetName,
		Content:    req.Content,
		Context:    req.Context,
		Status:     models.OutboundDraft,
	}

	id, err := s.db.InsertOutboundMessage(msg)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"message_id": id, "status": "draft"})
}

func (s *Server) approveOutbound(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.UpdateOutboundStatus(id, models.OutboundApproved); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "approved", "message": "Đã duyệt! Tin nhắn sẽ được gửi tự động."})
}

func (s *Server) rejectOutbound(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.UpdateOutboundStatus(id, models.OutboundRejected); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "rejected"})
}

func (s *Server) editOutbound(c *fiber.Ctx) error {
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
	if err := s.db.UpdateOutboundContent(id, req.Content); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

func (s *Server) deleteOutbound(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := s.db.DeleteOutbound(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (s *Server) deleteAllOutboundComments(c *fiber.Ctx) error {
	count, err := s.db.DeleteAllOutboundComments()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	log.Printf("[API] Reset all outbound comments: %d deleted", count)
	return c.JSON(fiber.Map{"ok": true, "deleted": count})
}

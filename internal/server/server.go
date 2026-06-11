package server

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/agentloop"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/mailer"
	serveragent "github.com/thg/scraper/internal/server/agent"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace"
)

// Config holds security-sensitive configuration for the API server.
type Config struct {
	Port           int
	JWTSecret      string
	AllowedOrigins string
	ChromePath     string
	ProfileDir     string
	Headless       bool
	ServerHost     string
	SSHPort        int

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	Mailer mailer.Config

	Notifier func(string)

	// Telegram integration feature flags + bot identity, surfaced to the integrations
	// control-plane handlers (default-safe; actions off by default).
	TelegramBotToken            string
	TelegramBotEnabled          bool
	TelegramNotifyEnabled       bool
	TelegramActionsEnabled      bool
	TelegramWebhookSecret       string
	TelegramAllowGlobalFallback bool
}

// Server provides the REST API and serves the Web UI.
type Server struct {
	app          *fiber.App
	db           *store.Store
	jobStore     *jobs.Store
	agent        *ai.Agent
	aiClass      *ai.MessageGenerator
	wsHub        *serveragent.WSHub
	workspace    *workspace.Manager
	sessionReg   *session.Registry
	agentHandler *agentloop.Handler
	port         int
	cfg          Config
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
		wsHub:     serveragent.NewWSHub(),
		workspace: wm,
	}
	s.registerRoutes()
	return s
}

func (s *Server) SetSessionRegistry(r *session.Registry) {
	s.sessionReg = r
}

func (s *Server) SetAgentHandler(h *agentloop.Handler) {
	s.agentHandler = h
}

func (s *Server) SetUniversalClassifier(mg *ai.MessageGenerator) {
	s.aiClass = mg
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

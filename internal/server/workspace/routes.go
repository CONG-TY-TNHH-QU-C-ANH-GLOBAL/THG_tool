package workspace

import (
	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
	"github.com/thg/scraper/internal/store"
	browserworkspace "github.com/thg/scraper/internal/workspace"
)

type Deps struct {
	DB        *store.Store
	Workspace *browserworkspace.Manager
}

type Handler struct {
	db        *store.Store
	workspace *browserworkspace.Manager
}

func NewHandler(deps Deps) *Handler {
	return &Handler{db: deps.DB, workspace: deps.Workspace}
}

func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := NewHandler(deps)
	group.Get("/accounts", h.getAccounts)
	group.Post("/accounts", h.addAccount)
	// Deterministic ExecutionContext: per-member Default Account.
	group.Get("/execution-context", h.getExecutionContext)
	group.Put("/execution-context", h.setExecutionContext)
	group.Put("/accounts/:id/status", adminOnly, h.updateAccountStatus)
	group.Put("/accounts/:id/cookies", adminOnly, h.updateAccountCookies)
	group.Delete("/accounts/:id", adminOnly, h.deleteAccount)
	// Admin safety switch over task assignment (PR-2b) — pauses queueing,
	// never touches the device (device privacy stays with the owner).
	group.Put("/accounts/:id/pause", adminOnly, h.pauseAccountAssignment)
	group.Put("/accounts/:id/resume", adminOnly, h.resumeAccountAssignment)

	group.Get("/browser/workspaces", h.workspaceList)
	group.Post("/browser/workspaces/new", h.workspaceNew)
	group.Post("/browser/workspaces/:id/start", h.workspaceStart)
	group.Post("/browser/workspaces/:id/stop", h.workspaceStop)
	group.Post("/browser/workspaces/:id/navigate", h.workspaceNavigate)
	group.Post("/browser/workspaces/:id/sync-session", h.workspaceSyncSession)
	group.Post("/browser/workspaces/:id/set-logged-in", h.workspaceSetLoggedIn)
	group.Post("/browser/workspaces/:id/resolve-checkpoint", h.resolveCheckpoint)
	group.Get("/browser/checkpoints", h.listCheckpoints)
}

func ScreenProxyHandler(deps Deps) func(*fiberws.Conn) {
	return NewHandler(deps).screenProxyHandler()
}

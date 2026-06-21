package superadmin

import "github.com/gofiber/fiber/v2"

// RegisterRoutes mounts the founder-only superadmin endpoints under the given
// parent router, preserving the exact effective paths and middleware chain that
// previously lived in org.Routes: parent is the protected /api group (RequireAuth
// → tenantReady → freshOrg), and founderOnly gates the /superadmin subgroup.
//
// Effective paths (unchanged by the extraction): /api/superadmin/...
func RegisterRoutes(parent fiber.Router, deps Deps, founderOnly fiber.Handler) {
	h := NewHandler(deps)

	g := parent.Group("/superadmin", founderOnly)
	g.Get("/orgs", h.listOrgs)
	g.Put("/orgs/:id", h.adminUpdateOrg)
	g.Delete("/orgs/:id", h.superAdminDeleteOrg)
	g.Get("/accounts", h.superAdminAccounts)
	g.Delete("/accounts/:id", h.superAdminDeleteAccount)
	g.Get("/users", h.superAdminUsers)
	g.Delete("/users/:id", h.superAdminDeleteUser)
	g.Get("/sessions", h.superAdminSessions)
	g.Delete("/sessions/:id", h.superAdminTerminateSession)
	g.Post("/query", h.superAdminQuery)
	// Single-purpose diagnostic surface for the redirected_feed
	// investigation. NOT a tab in the CRUD sense — a focused operational
	// action: "tell me why account X is failing" + "let me reset its
	// risk after I fix the root cause". See
	// project_runtime_control_plane memory for the broader EXP track.
	g.Get("/accounts/:id/diagnostic", h.superAdminAccountDiagnostic)
	g.Post("/accounts/:id/reset-risk", h.superAdminAccountResetRisk)
	// Comment verification forensics: why did these target URLs verify / fail?
	g.Get("/comment-forensics", h.superAdminCommentForensics)
	g.Post("/comment-forensics", h.superAdminCommentForensics)
}

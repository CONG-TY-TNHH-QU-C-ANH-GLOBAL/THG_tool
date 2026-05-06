package skills

import "github.com/gofiber/fiber/v2"

// Routes registers tenant skill catalog and execution audit endpoints.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	group.Get("/skills", list(deps))
	group.Get("/skills/executions", executions(deps))
	group.Put("/skills/:id/enable", adminOnly, setEnabled(deps, true))
	group.Put("/skills/:id/disable", adminOnly, setEnabled(deps, false))
}

// AdminRoutes registers platform/admin skill inspection endpoints.
func AdminRoutes(group fiber.Router, deps Deps) {
	group.Get("/skills", all(deps))
}

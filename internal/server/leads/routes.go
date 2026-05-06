package leads

import (
	"github.com/gofiber/fiber/v2"
)

// Routes registers all leads, niches, posts, groups, and jobs endpoints.
// group should be the protected router (already JWT-authenticated).
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	// Leads
	group.Get("/leads", getLeads(deps))
	group.Delete("/leads/all", adminOnly, deleteAllLeads(deps))
	group.Delete("/leads/:id", deleteLead(deps))

	// Niches
	group.Get("/niches", getNiches(deps))
	group.Post("/niches", adminOnly, addNiche(deps))
	group.Delete("/niches/:slug", adminOnly, deleteNiche(deps))

	// Posts
	group.Get("/posts", getPosts(deps))
	group.Delete("/posts/all", adminOnly, deleteAllPosts(deps))
	group.Delete("/posts/:id", adminOnly, deletePost(deps))

	// Groups
	group.Get("/groups", getGroups(deps))
	group.Post("/groups", adminOnly, addGroup(deps))
	group.Put("/groups/:id/toggle", adminOnly, toggleGroup(deps))
	group.Delete("/groups/:id", adminOnly, deleteGroup(deps))

	// Jobs
	group.Get("/jobs", getJobs(deps))
	group.Post("/jobs", createJob(deps))
	group.Delete("/jobs/:id", adminOnly, cancelJob(deps))
}

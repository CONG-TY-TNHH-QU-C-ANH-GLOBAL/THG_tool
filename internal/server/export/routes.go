// Package export exposes the additive, read-only Knowledge OS export
// endpoint that the THG VectorHub `thgtool` connector pulls from
// (contract: EXPORT_ENDPOINT_CONTRACT.md in the vectorhub repo).
//
// It is intentionally separate from internal/server/knowledge: that
// package is the JWT + org-scoped operator CRUD surface, whereas this one
// is a single service-key-authenticated, cross-service export feeding the
// derived-index platform (platform invariant I3: pull-only — the platform
// never writes back through here).
//
// The route is mounted by the composition root ONLY when both the service
// key and the export org id are configured, so an unconfigured production
// deployment exposes nothing new.
package export

import (
	"crypto/subtle"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// Deps is the dependency bundle for the export handler. Fields are set
// once at boot from environment config in the composition root.
type Deps struct {
	DB         *store.Store
	ServiceKey string // shared secret; clients present it in X-Service-Key
	OrgID      int64  // the single org whose assets this endpoint exports
}

const serviceKeyHeader = "X-Service-Key"

// Routes registers GET /export/knowledge-assets under group, gated by a
// constant-time service-key check. The caller passes the /api group, so
// the effective path is /api/export/knowledge-assets — matching what the
// vectorhub connector requests. No JWT chain: the service key is the only
// credential, and the endpoint reads a fixed org.
func Routes(group fiber.Router, deps Deps) {
	h := &handler{deps: deps}
	group.Get("/export/knowledge-assets", requireServiceKey(deps.ServiceKey), h.exportAssets)
}

type handler struct{ deps Deps }

// requireServiceKey compares the request's X-Service-Key against the
// configured key in constant time to avoid a timing oracle. An empty
// configured key rejects everything: the route should not have been
// mounted in that case, so this is defence in depth.
func requireServiceKey(want string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		got := c.Get(serviceKeyHeader)
		if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid service key")
		}
		return c.Next()
	}
}

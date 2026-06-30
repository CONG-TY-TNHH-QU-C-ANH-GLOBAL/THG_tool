package agent

import (
	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/server/agent/connector"
)

// The connector lifecycle handlers live in the connector subpackage. These thin
// delegators keep the composition root (router.go) registering them through the
// agent package unchanged — the parent composes/delegates, the implementation and
// its own Handler live in internal/server/agent/connector.

// LocalConnectorDeps is the dependency set for the dashboard-authenticated
// connector management routes (pairing-code + remote input commands).
type LocalConnectorDeps = connector.LocalConnectorDeps

// LocalConnectorRoutes registers the dashboard-authenticated connector routes.
func LocalConnectorRoutes(group fiber.Router, deps LocalConnectorDeps, adminOnly fiber.Handler) {
	connector.LocalConnectorRoutes(group, deps, adminOnly)
}

// LocalConnectorPairingRoutes registers the first-time pairing-code endpoint.
func LocalConnectorPairingRoutes(group fiber.Router, deps LocalConnectorDeps, pairingLimiter fiber.Handler) {
	connector.LocalConnectorPairingRoutes(group, deps, pairingLimiter)
}

package platform

import (
	"github.com/gofiber/fiber/v2"
	platformsvc "github.com/thg/scraper/internal/platform/services"
	"github.com/thg/scraper/internal/platform/services/adapters"
	"github.com/thg/scraper/internal/platform/services/contracts"
)

// Handler serves the platform service registry endpoints.
type Handler struct {
	deps     Deps
	registry *platformsvc.Registry
}

// listServices handles GET /api/platform/services.
//
// It composes the adapter (storage -> domain) and each service's resolvers
// (domain -> contract). No business branching lives in this handler — every
// semantic decision is delegated to a resolver (mandates 2 & 8).
func (h *Handler) listServices(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)

	uc, err := adapters.LoadUserContext(h.deps.DB, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not load user context"})
	}

	resolvers := h.registry.All()
	services := make([]contracts.PlatformService, 0, len(resolvers))
	for _, svc := range resolvers {
		services = append(services, project(svc, uc))
	}

	return c.JSON(contracts.Envelope{
		ContractVersion: contracts.ContractVersion,
		Services:        services,
	})
}

// project runs a service's four resolvers and assembles the cross-boundary
// contract. Pure composition — no IO, no branching on storage fields.
func project(svc platformsvc.Resolver, uc contracts.UserContext) contracts.PlatformService {
	d := svc.Descriptor()
	status := svc.ResolveStatus(uc)
	workspace := svc.ResolveWorkspace(uc)
	capabilities := svc.ResolveCapabilities(uc)
	access := svc.ResolveAccess(uc)

	traces := make([]contracts.ResolutionTrace, 0, 2)
	if workspace.Trace != nil {
		traces = append(traces, *workspace.Trace)
	}
	if access.Trace != nil {
		traces = append(traces, *access.Trace)
	}

	return contracts.PlatformService{
		Slug:             d.Slug,
		Label:            d.PublicLabel,
		ServiceVersion:   d.Version,
		Descriptor:       d,
		Status:           status,
		WorkspaceState:   workspace.State,
		WorkspaceID:      workspace.WorkspaceID,
		Access:           access.Access,
		AccessReason:     access.Reason,
		Reason:           workspace.Reason,
		Capabilities:     capabilities,
		ResolutionTraces: traces,
	}
}

package resolver

import "github.com/thg/scraper/internal/platform/services/contracts"

// Alibaba1688Resolver registers 1688 sourcing as a Coming Soon service.
// Same shape as TaobaoResolver — status="unavailable" so the card is
// listed but not interactive. This is the visual lever that signals
// "THG is a multi-service platform" on the post-login services page.
type Alibaba1688Resolver struct{}

func NewAlibaba1688Resolver() *Alibaba1688Resolver { return &Alibaba1688Resolver{} }

func (Alibaba1688Resolver) Descriptor() contracts.ServiceDescriptor {
	return contracts.ServiceDescriptor{
		Slug:         "alibaba_1688",
		InternalName: "1688-sourcing",
		PublicLabel:  "1688 Sourcing",
		Category:     "automation",
		RolloutStage: "alpha",
		Availability: "public",
		Version:      1,
		DisplayOrder: 30,
	}
}

func (Alibaba1688Resolver) ResolveStatus(contracts.UserContext) contracts.ServiceStatus {
	return contracts.StatusUnavailable
}

func (Alibaba1688Resolver) ResolveWorkspace(uc contracts.UserContext) contracts.WorkspaceResolution {
	return contracts.WorkspaceResolution{
		State: contracts.WorkspaceNone,
		Trace: &contracts.ResolutionTrace{
			Source:          "stub",
			Resolver:        "alibaba_1688.ResolveWorkspace",
			Confidence:      "authoritative",
			AuthoritativeAt: uc.ResolvedAt,
		},
	}
}

func (Alibaba1688Resolver) ResolveCapabilities(contracts.UserContext) contracts.ServiceCapabilities {
	return contracts.ServiceCapabilities{
		MultiWorkspace:    false,
		BrowserAutomation: true,
		AIAgents:          true,
	}
}

func (Alibaba1688Resolver) ResolveAccess(uc contracts.UserContext) contracts.AccessResolution {
	return contracts.AccessResolution{
		Access: contracts.AccessGranted,
		Trace: &contracts.ResolutionTrace{
			Source:          "stub",
			Resolver:        "alibaba_1688.ResolveAccess",
			Confidence:      "authoritative",
			AuthoritativeAt: uc.ResolvedAt,
		},
	}
}

package resolver

import "github.com/thg/scraper/internal/platform/services/contracts"

// TaobaoResolver registers Taobao as a Coming Soon service so the platform
// services page reflects the multi-platform target (FB + Taobao + 1688)
// instead of feeling like a single-service Facebook tool. The actual
// automation backend is not implemented yet — ResolveStatus returns
// "unavailable" which makes the UI render a "Sắp ra mắt" card with no
// interactive entry.
type TaobaoResolver struct{}

func NewTaobaoResolver() *TaobaoResolver { return &TaobaoResolver{} }

func (TaobaoResolver) Descriptor() contracts.ServiceDescriptor {
	return contracts.ServiceDescriptor{
		Slug:         "taobao",
		InternalName: "taobao-sourcing",
		PublicLabel:  "Taobao Sourcing",
		Category:     "automation",
		RolloutStage: "alpha",
		Availability: "public",
		Version:      1,
		DisplayOrder: 20,
	}
}

func (TaobaoResolver) ResolveStatus(contracts.UserContext) contracts.ServiceStatus {
	return contracts.StatusUnavailable
}

func (TaobaoResolver) ResolveWorkspace(uc contracts.UserContext) contracts.WorkspaceResolution {
	return contracts.WorkspaceResolution{
		State: contracts.WorkspaceNone,
		Trace: &contracts.ResolutionTrace{
			Source:          "stub",
			Resolver:        "taobao.ResolveWorkspace",
			Confidence:      "authoritative",
			AuthoritativeAt: uc.ResolvedAt,
		},
	}
}

func (TaobaoResolver) ResolveCapabilities(contracts.UserContext) contracts.ServiceCapabilities {
	return contracts.ServiceCapabilities{
		MultiWorkspace:    false,
		BrowserAutomation: true,
		AIAgents:          true,
	}
}

func (TaobaoResolver) ResolveAccess(uc contracts.UserContext) contracts.AccessResolution {
	return contracts.AccessResolution{
		Access: contracts.AccessGranted,
		Trace: &contracts.ResolutionTrace{
			Source:          "stub",
			Resolver:        "taobao.ResolveAccess",
			Confidence:      "authoritative",
			AuthoritativeAt: uc.ResolvedAt,
		},
	}
}

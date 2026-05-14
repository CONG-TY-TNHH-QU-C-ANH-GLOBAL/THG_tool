// Package resolver holds per-service resolvers. A resolver is a pure function
// set over a contracts.UserContext — deterministic, side-effect free, no IO,
// no DB writes. It does not call time.Now: "now" arrives via UserContext.ResolvedAt
// so the resolver stays deterministic. See BOUNDARIES.md § Resolver purity rule.
package resolver

import "github.com/thg/scraper/internal/platform/services/contracts"

// FacebookResolver resolves the Facebook Automation service for a user.
type FacebookResolver struct{}

// NewFacebookResolver returns the Facebook Automation resolver.
func NewFacebookResolver() *FacebookResolver { return &FacebookResolver{} }

func (FacebookResolver) Descriptor() contracts.ServiceDescriptor {
	return contracts.ServiceDescriptor{
		Slug:         "facebook",
		InternalName: "facebook-automation",
		PublicLabel:  "Facebook Automation",
		Category:     "automation",
		RolloutStage: "ga",
		Availability: "public",
		Version:      1,
		DisplayOrder: 10,
	}
}

func (FacebookResolver) ResolveStatus(contracts.UserContext) contracts.ServiceStatus {
	return contracts.StatusAvailable
}

func (FacebookResolver) ResolveWorkspace(uc contracts.UserContext) contracts.WorkspaceResolution {
	trace := &contracts.ResolutionTrace{
		Source:          "platform_org",
		Resolver:        "facebook.ResolveWorkspace",
		Confidence:      "authoritative",
		AuthoritativeAt: uc.ResolvedAt,
	}
	if uc.Org == nil || uc.Org.ID <= 0 {
		return contracts.WorkspaceResolution{State: contracts.WorkspaceNone, Trace: trace}
	}
	if !uc.Org.Active {
		return contracts.WorkspaceResolution{
			State:       contracts.WorkspaceSuspended,
			WorkspaceID: contracts.WorkspaceIDOf(uc.Org.ID),
			Reason:      "workspace is inactive",
			Trace:       trace,
		}
	}
	return contracts.WorkspaceResolution{
		State:       contracts.WorkspaceReady,
		WorkspaceID: contracts.WorkspaceIDOf(uc.Org.ID),
		Trace:       trace,
	}
}

func (FacebookResolver) ResolveCapabilities(contracts.UserContext) contracts.ServiceCapabilities {
	// Capability = "the FB service supports this feature". NOT a permission.
	// Whether the user may run it is ResolveAccess + (future) RBAC.
	return contracts.ServiceCapabilities{
		MultiWorkspace:    false,
		BrowserAutomation: true,
		AIAgents:          true,
	}
}

func (FacebookResolver) ResolveAccess(uc contracts.UserContext) contracts.AccessResolution {
	trace := &contracts.ResolutionTrace{
		Source:          "platform_org",
		Resolver:        "facebook.ResolveAccess",
		Confidence:      "authoritative",
		AuthoritativeAt: uc.ResolvedAt,
	}
	if !uc.Authenticated {
		return contracts.AccessResolution{
			Access: contracts.AccessAdminBlocked,
			Reason: "not authenticated",
			Trace:  trace,
		}
	}
	return contracts.AccessResolution{Access: contracts.AccessGranted, Trace: trace}
}

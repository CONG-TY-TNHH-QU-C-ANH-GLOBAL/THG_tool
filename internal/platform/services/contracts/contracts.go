// Package contracts defines the platform service domain contracts.
//
// These are DOMAIN types, not ORM rows. They are the cross-boundary shape
// returned by GET /api/platform/services and consumed by the frontend.
// Storage shapes (models.Organization, models.User) never appear here — the
// adapters package transforms storage into these domain types.
//
// Mirror of frontend/src/platform/services/types.ts. See also:
//   - specs/domains/platform-foundation/DOMAIN.md
//   - frontend/src/platform/BOUNDARIES.md
package contracts

import "fmt"

// ContractVersion is the version of the GET /api/platform/services envelope.
// Bump on a breaking change to the response shape so stale clients (mobile
// apps, the Chrome extension, cached metadata) can negotiate or refuse.
const ContractVersion = 1

// ── State axes — orthogonal. Each answers exactly one question. ──

// ServiceStatus: is the service offered to this user at all?
type ServiceStatus string

const (
	StatusAvailable   ServiceStatus = "available"
	StatusUnavailable ServiceStatus = "unavailable"
	StatusSuspended   ServiceStatus = "suspended"
)

// WorkspaceState: does the user's workspace exist and is it operational?
type WorkspaceState string

const (
	WorkspaceNone         WorkspaceState = "none"
	WorkspaceInitializing WorkspaceState = "initializing"
	WorkspaceReady        WorkspaceState = "ready"
	WorkspaceSuspended    WorkspaceState = "suspended"
)

// ServiceAccess: may the user actually enter / operate the service right now?
type ServiceAccess string

const (
	AccessGranted        ServiceAccess = "granted"
	AccessInviteRequired ServiceAccess = "invite_required"
	AccessBillingBlocked ServiceAccess = "billing_blocked"
	AccessRegionLocked   ServiceAccess = "region_locked"
	AccessAdminBlocked   ServiceAccess = "admin_blocked"
)

// ServiceDescriptor — static metadata, independent of per-user state.
type ServiceDescriptor struct {
	Slug         string `json:"slug"`
	InternalName string `json:"internalName"`
	PublicLabel  string `json:"publicLabel"`
	Category     string `json:"category"`
	RolloutStage string `json:"rolloutStage"`
	Availability string `json:"availability"`
	Version      int    `json:"version"`
	DisplayOrder int    `json:"displayOrder"`
}

// ServiceCapabilities — what the service CAN do in this user's context.
// Capability != Access != Permission. See BOUNDARIES.md.
type ServiceCapabilities struct {
	MultiWorkspace    bool `json:"multiWorkspace"`
	BrowserAutomation bool `json:"browserAutomation"`
	AIAgents          bool `json:"aiAgents"`
}

// ResolutionTrace — internal provenance metadata. Never load-bearing for UI.
type ResolutionTrace struct {
	Source          string `json:"source"`
	Resolver        string `json:"resolver"`
	Confidence      string `json:"confidence"` // legacy | authoritative | cached | stale
	AuthoritativeAt int64  `json:"authoritativeAt,omitempty"`
}

// WorkspaceResolution — output of a service's ResolveWorkspace.
type WorkspaceResolution struct {
	State       WorkspaceState
	WorkspaceID string
	Reason      string
	Trace       *ResolutionTrace
}

// AccessResolution — output of a service's ResolveAccess.
type AccessResolution struct {
	Access ServiceAccess
	Reason string
	Trace  *ResolutionTrace
}

// PlatformService — the cross-boundary contract. Mirror of the FE type.
type PlatformService struct {
	Slug             string              `json:"slug"`
	Label            string              `json:"label"`
	ServiceVersion   int                 `json:"serviceVersion"`
	Descriptor       ServiceDescriptor   `json:"descriptor"`
	Status           ServiceStatus       `json:"status"`
	WorkspaceState   WorkspaceState      `json:"workspaceState"`
	WorkspaceID      string              `json:"workspaceId,omitempty"`
	Access           ServiceAccess       `json:"access"`
	AccessReason     string              `json:"accessReason,omitempty"`
	Reason           string              `json:"reason,omitempty"`
	Capabilities     ServiceCapabilities `json:"capabilities"`
	ResolutionTraces []ResolutionTrace   `json:"resolutionTraces,omitempty"`
}

// Envelope — the GET /api/platform/services response shape. The version field
// lets stale clients negotiate.
type Envelope struct {
	ContractVersion int               `json:"contractVersion"`
	Services        []PlatformService `json:"services"`
}

// ── Domain entities (post-adapter, pre-resolver) ──

// OrgContext is the domain view of an organization. Produced by the adapters
// package from models.Organization — resolvers consume this, never the ORM row.
type OrgContext struct {
	ID       int64
	Name     string
	PlanTier string
	Active   bool
}

// UserContext is the domain view of an authenticated user. Produced by the
// adapters package. Resolvers are pure functions over this — they never touch
// *store.Store and never call time.Now (ResolvedAt carries "now" so resolvers
// stay deterministic; see BOUNDARIES.md § Resolver purity rule).
type UserContext struct {
	UserID        int64
	Role          string
	Authenticated bool
	Org           *OrgContext // nil if the user has no organization yet
	ResolvedAt    int64       // epoch ms — when the adapter loaded this context
}

// WorkspaceIDOf encodes the canonical workspace ID. Storage uses numeric org
// IDs; the contract boundary uses the "ws_<n>" prefixed string form.
func WorkspaceIDOf(orgID int64) string {
	if orgID <= 0 {
		return ""
	}
	return fmt.Sprintf("ws_%d", orgID)
}

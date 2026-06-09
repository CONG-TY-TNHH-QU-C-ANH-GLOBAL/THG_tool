package models

// Account readiness matrix (FB Automation Reliability PR-D). A per-account,
// per-capability projection so every mission/action UI can show "can this account
// do X, and if not, why" instead of guessing. Reasons are the SAME typed codes the
// gates emit (connector readiness + behaviour caps), so the matrix and the gates
// never disagree. See specs/FACEBOOK_AUTOMATION_RELIABILITY_TRACK.md (PR-D).

// Capability names (closed set).
const (
	CapabilityCrawl   = "crawl"
	CapabilityComment = "comment"
	CapabilityInbox   = "inbox"
	CapabilityPost    = "post"
)

// CapabilityReadiness is whether one account can perform one capability now.
type CapabilityReadiness struct {
	Capability string   `json:"capability"`
	Can        bool     `json:"can"`
	Reasons    []string `json:"reasons"` // typed reason codes blocking it; empty when Can
}

// AccountReadiness is the readiness of one account across all capabilities.
type AccountReadiness struct {
	AccountID        int64                 `json:"account_id"`
	AccountName      string                `json:"account_name"`
	AssignedUserName string                `json:"assigned_user_name"` // who manages this account ("" if unassigned)
	FBUserID         string                `json:"fb_user_id"`
	FBDisplayName    string                `json:"fb_display_name"`
	ConnectorID      int64                 `json:"connector_id"`
	MachineLabel     string                `json:"machine_label"`      // human label typed at pairing (PR-C)
	BrowserProfileID string                `json:"browser_profile_id"` // stable per-Chrome-profile id (PR-C)
	ExtensionVersion string                `json:"extension_version"`
	Capabilities     []CapabilityReadiness `json:"capabilities"`
	RequiredAction   string                `json:"required_action"` // top actionable hint, "" when fully ready
}

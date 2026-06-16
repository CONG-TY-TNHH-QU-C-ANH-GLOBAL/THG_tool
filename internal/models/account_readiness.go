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

// Account-level EXECUTABILITY reason codes (P1.3E, closed set). These describe whether the
// REQUESTER can run automation on this account RIGHT NOW with their OWN live connector — a
// stricter, requester-scoped signal than the org-wide per-capability `can`. `ready` means
// executable; every other code is a distinct not-executable state the UI can label.
const (
	ExecReasonReady           = "ready"
	ExecReasonNoConnector     = "no_connector"     // no requester-owned connector paired to this account
	ExecReasonConnectorStale  = "connector_stale"  // owned connector paired but heartbeat stale/offline
	ExecReasonPairingPending  = "pairing_pending"  // owned connector online but not yet logged-in/identity-read
	ExecReasonIdentityMismatch = "identity_mismatch" // live connector logged into a DIFFERENT fb identity
	ExecReasonSessionBlocked  = "session_blocked"  // session at login wall / checkpoint / logged out
	ExecReasonAccountBlocked  = "account_blocked"  // account suspended/banned or Verified-Actor blocked
	ExecReasonNotControllable = "not_controllable" // requester may VIEW but not CONTROL this account
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
	ExtensionVersion string `json:"extension_version"`
	// ExtensionVersionState: latest | update_available | update_required |
	// unsupported (PR-4). update_available still runs (soft warning);
	// the blocking states also surface as capability reasons.
	ExtensionVersionState string                `json:"extension_version_state"`
	Capabilities          []CapabilityReadiness `json:"capabilities"`
	RequiredAction        string                `json:"required_action"` // top actionable hint, "" when fully ready

	// P1.3E requester-scoped executability (additive — capabilities[].can stays the org-wide
	// projection other consumers depend on). `executable` is the ONLY field a UI may use to
	// show green "Sẵn sàng": it is true only when the REQUESTER can run automation on THIS
	// account right now via their OWN live, identity-matched, logged-in connector on a
	// controllable, unblocked account. The booleans below decompose it for diagnostics.
	Configured          bool   `json:"configured"`            // account has a bound fb identity
	ControlAllowed      bool   `json:"control_allowed"`       // requester is assigned this account (not mere visibility)
	Paired              bool   `json:"paired"`                // a requester-owned connector is bound to this account
	ConnectorOnline     bool   `json:"connector_online"`      // that connector heartbeated within TTL
	HeartbeatFresh      bool   `json:"heartbeat_fresh"`       // alias of connector_online (90s TTL); explicit for clarity
	LiveIdentityMatched bool   `json:"live_identity_matched"` // live connector fb_user_id == account.fb_user_id
	SessionUsable       bool   `json:"session_usable"`        // stream is facebook_logged_in (not login wall/checkpoint)
	Executable          bool   `json:"executable"`            // ALL of the above + not blocked → runnable now
	ExecReasonCode      string `json:"exec_reason_code"`      // typed: ready|no_connector|connector_stale|...
	ExecReasonMessage   string `json:"exec_reason_message"`   // short customer-facing VN message
}

package copilot

import "github.com/thg/scraper/internal/ai"

// Brain* DTOs: the wire shapes exchanged with the local Python planner
// sidecar. The sidecar returns a schema-first action plan; Go validates
// and executes. Transport lives in brain_client.go; validation in
// brain_plan_validator.go; arg prep in brain_action_prep.go.

type BrainPlanRequest struct {
	OrgID             int64               `json:"org_id"`
	Source            string              `json:"source"`
	Prompt            string              `json:"prompt"`
	BusinessProfile   *ai.BusinessProfile `json:"business_profile,omitempty"`
	SelectedAccountID int64               `json:"selected_account_id,omitempty"`
	Accounts          []BrainAccount      `json:"accounts"`
	DataSummaries     BrainDataSummaries  `json:"data_summaries"`
	ToolCapabilities  []string            `json:"tool_capabilities"`
	Policy            BrainPolicy         `json:"policy"`
}

type BrainAccount struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Platform      string `json:"platform"`
	Status        string `json:"status"`
	Email         string `json:"email,omitempty"`
	FBUserID      string `json:"fb_user_id,omitempty"`
	FBDisplayName string `json:"fb_display_name,omitempty"`
	FBUsername    string `json:"fb_username,omitempty"`
	Ready         bool   `json:"ready"`
}

type BrainDataSummaries struct {
	PrivateFilesSummary string `json:"private_files_summary,omitempty"`
	DataSourcesSummary  string `json:"data_sources_summary,omitempty"`
}

type BrainPolicy struct {
	DefaultOutboundMode string   `json:"default_outbound_mode"`
	MaxItemsCap         int      `json:"max_items_cap"`
	BrowserRequiredFor  []string `json:"browser_required_for"`
}

type BrainPlanResponse struct {
	DomainScope      string                `json:"domain_scope"`
	Intent           string                `json:"intent"`
	Decision         string                `json:"decision"`
	Confidence       float64               `json:"confidence"`
	ResponseSummary  string                `json:"response_summary"`
	MarketSignalGate BrainMarketSignalGate `json:"market_signal_gate"`
	Actions          []BrainAction         `json:"actions"`
}

type BrainMarketSignalGate struct {
	TargetRole      string   `json:"target_role"`
	PositiveSignals []string `json:"positive_signals"`
	NegativeSignals []string `json:"negative_signals"`
	RejectRules     []string `json:"reject_rules"`
	MinConfidence   float64  `json:"min_confidence"`
}

type BrainAction struct {
	Tool            string          `json:"tool"`
	Args            map[string]any  `json:"args"`
	Reason          string          `json:"reason"`
	Evidence        []string        `json:"evidence"`
	RequiresBrowser bool            `json:"requires_browser"`
	RequiresProfile bool            `json:"requires_profile"`
	Recurrence      BrainRecurrence `json:"recurrence"`
}

type BrainRecurrence struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

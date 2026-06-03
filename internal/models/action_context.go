package models

// ActionSource identifies what initiated an execution. The deterministic
// ExecutionContext is designed to extend into a CampaignContext: every source
// resolves to the SAME ActionContext shape, so the queue/execution path depends
// ONLY on ActionContext. Adding a new source (campaign, AI agent, scheduled job,
// workflow) is ADDITIVE — it never requires changing the execution path.
// Channel-independent: Campaign must NOT depend on Facebook (FB is one channel).
type ActionSource string

const (
	ActionSourceManual    ActionSource = "manual"
	ActionSourceCampaign  ActionSource = "campaign"
	ActionSourceAIAgent   ActionSource = "ai_agent"
	ActionSourceScheduled ActionSource = "scheduled"
	ActionSourceWorkflow  ActionSource = "workflow"
)

// ActionContext is the campaign-ready value object that fully describes
// WHO / WHAT-source / WHERE an outbound action runs under. It is resolved
// deterministically BEFORE queueing (no worker guessing). Future resolvers
// (ResolveCampaignActionContext, ...) produce the same shape, so the queue path
// is source-agnostic.
type ActionContext struct {
	OrgID             int64
	Source            ActionSource
	ExecutionSourceID int64 // trace id of the originating session/campaign/job (0 = none yet)
	InitiatorUserID   int64 // the MEMBER who owns the execution (= created_by, IMMUTABLE)
	AccountID         int64 // resolved owned account
	ConnectorID       int64 // OPTIONAL: 0 = connectorless (future email / telegram / scheduled)
	CampaignID        int64 // 0 = not part of a campaign
}

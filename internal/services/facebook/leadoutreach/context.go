package leadoutreach

import (
	"context"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/services/facebook/commenting"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// Context is the resolved per-run config for the per-lead pipeline (S107). Its
// fields are unexported so the method bodies stay byte-identical to the original;
// the composition root builds it via New(Config{...}).
type Context struct {
	outbound         OutboundRecorder
	coverage         LeadCoverageReader
	lifecycle        LeadLifecycleReader
	contacts         facebook.ContactDirectory
	promptLog        commenting.SystemPromptLogInserter
	msgGen           *ai.MessageGenerator
	knowledgeBuilder *knowledgeRuntime.Builder
	msgType          string
	orgID            int64
	accountID        int64
	actx             models.ActionContext
	template         string
	businessContext  string
	reasoningMode    string
	reasoningProfile *ai.BusinessProfile
	commentIdentity  models.CompanyIdentity
	commentPolicies  ai.CommentPolicies
	coveragePolicy   models.CoveragePolicy
}

// Config carries the resolved per-run values from the composition root (cmd builds
// the store-backed adapters + resolves the org-scoped values, then calls New). The
// exported fields map 1:1 onto Context's unexported fields.
type Config struct {
	Outbound         OutboundRecorder
	Coverage         LeadCoverageReader
	Lifecycle        LeadLifecycleReader
	Contacts         facebook.ContactDirectory
	PromptLog        commenting.SystemPromptLogInserter
	MsgGen           *ai.MessageGenerator
	KnowledgeBuilder *knowledgeRuntime.Builder
	MsgType          string
	OrgID            int64
	AccountID        int64
	Actx             models.ActionContext
	Template         string
	BusinessContext  string
	ReasoningMode    string
	ReasoningProfile *ai.BusinessProfile
	CommentIdentity  models.CompanyIdentity
	CommentPolicies  ai.CommentPolicies
	CoveragePolicy   models.CoveragePolicy
}

// New builds a *Context from cfg, copying each Config field into the unexported
// Context field 1:1.
func New(cfg Config) *Context {
	return &Context{
		outbound:         cfg.Outbound,
		coverage:         cfg.Coverage,
		lifecycle:        cfg.Lifecycle,
		contacts:         cfg.Contacts,
		promptLog:        cfg.PromptLog,
		msgGen:           cfg.MsgGen,
		knowledgeBuilder: cfg.KnowledgeBuilder,
		msgType:          cfg.MsgType,
		orgID:            cfg.OrgID,
		accountID:        cfg.AccountID,
		actx:             cfg.Actx,
		template:         cfg.Template,
		businessContext:  cfg.BusinessContext,
		reasoningMode:    cfg.ReasoningMode,
		reasoningProfile: cfg.ReasoningProfile,
		commentIdentity:  cfg.CommentIdentity,
		commentPolicies:  cfg.CommentPolicies,
		coveragePolicy:   cfg.CoveragePolicy,
	}
}

// ProcessLead runs the full per-lead pipeline (target → coverage → generate
// → reason → quality screen → queue), mutating st. It returns a non-nil error ONLY
// for a hard store failure that aborts the run (original `return "", err`); soft
// outcomes are recorded as skips.
func (c *Context) ProcessLead(ctx context.Context, lead models.Lead, st *State) error {
	targetURL, skipReason := facebook.ResolveOutboundTargetURL(lead, c.msgType)
	if skipReason != "" {
		st.recordSkip(skipReason, lead.ID)
		return nil
	}

	persona, covSkip := c.coverageGate(ctx, lead)
	if covSkip != "" {
		st.recordSkip(covSkip, lead.ID)
		return nil
	}

	content, retrievalID, genSkip, genErr := c.prepareOutreachContent(ctx, lead, targetURL, persona)
	if genErr != nil {
		st.lastGenErr = genErr
	}
	if genSkip != "" {
		st.recordSkip(genSkip, lead.ID)
		return nil
	}

	if c.msgType == "comment" {
		cleaned, qSkip := facebook.ScreenCommentQuality(content, c.commentIdentity)
		if qSkip != "" {
			st.recordSkip(qSkip, lead.ID)
			return nil
		}
		content = cleaned
	}

	return c.queueMessage(ctx, lead, targetURL, content, retrievalID, st)
}

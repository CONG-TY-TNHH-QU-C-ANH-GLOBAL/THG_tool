package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// leadOutreachState accumulates one queueLeadOutreach pass's counters + diagnostics.
type leadOutreachState struct {
	queued        int
	skipped       int
	approvedCount int
	scanned       int
	skipReasons   map[string]int
	// skipSamples keeps up to 5 sample lead IDs per skip reason (diagnosability).
	skipSamples map[string][]int64
	lastGenErr  error
	// riskBlock* capture the last risk_ceiling_exceeded deny for the response.
	riskBlockSeen    bool
	riskBlockRisk    float64
	riskBlockCeiling float64
}

func newLeadOutreachState() *leadOutreachState {
	return &leadOutreachState{
		skipReasons: map[string]int{},
		skipSamples: map[string][]int64{},
	}
}

func (s *leadOutreachState) recordSkip(reason string, leadID int64) {
	s.skipped++
	s.skipReasons[reason]++
	if leadID > 0 && len(s.skipSamples[reason]) < 5 {
		s.skipSamples[reason] = append(s.skipSamples[reason], leadID)
	}
}

// leadOutreachContext is the resolved per-run config for the per-lead pipeline (S107).
type leadOutreachContext struct {
	db               *store.Store
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
	coveragePolicy   models.CoveragePolicy
}

// buildLeadOutreachContext resolves the per-run config. Side-effect order matches
// the original preamble: businessContext → reasoning profile → comment identity.
func buildLeadOutreachContext(db *store.Store, msgGen *ai.MessageGenerator, msgType string, args map[string]any, orgID, accountID int64, actx models.ActionContext) *leadOutreachContext {
	businessContext := businessContextForOrg(db, orgID)
	knowledgeBuilder := knowledgeRuntime.NewBuilder(db.Knowledge())
	knowledgeBuilder.Recorder = db.Knowledge()
	knowledgeBuilder.TraceRec = db.Knowledge()

	reasoningMode := commentReasoningMode()
	var reasoningProfile *ai.BusinessProfile
	if reasoningMode != "off" {
		reasoningProfile = ai.LoadProfileForOrg(db, orgID)
	}

	var commentIdentity models.CompanyIdentity
	if msgType == "comment" {
		idProfile := reasoningProfile
		if idProfile == nil {
			idProfile = ai.LoadProfileForOrg(db, orgID)
		}
		commentIdentity = resolveCommentIdentity(db, orgID, actx.InitiatorUserID, accountID, idProfile, nil)
	}

	return &leadOutreachContext{
		db:               db,
		msgGen:           msgGen,
		knowledgeBuilder: knowledgeBuilder,
		msgType:          msgType,
		orgID:            orgID,
		accountID:        accountID,
		actx:             actx,
		template:         argString(args, "template"),
		businessContext:  businessContext,
		reasoningMode:    reasoningMode,
		reasoningProfile: reasoningProfile,
		commentIdentity:  commentIdentity,
		coveragePolicy:   models.DefaultCoveragePolicy(),
	}
}

// processOutreachLead runs the full per-lead pipeline (target → coverage → generate
// → reason → quality screen → queue), mutating st. It returns a non-nil error ONLY
// for a hard store failure that aborts the run (original `return "", err`); soft
// outcomes are recorded as skips.
func (c *leadOutreachContext) processOutreachLead(ctx context.Context, lead models.Lead, st *leadOutreachState) error {
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

	return c.queueOutreachMessage(ctx, lead, targetURL, content, retrievalID, st)
}

// coverageGate applies the multi-actor coverage policy for comment leads: returns
// the derived persona and "" to proceed, or a skip reason. A coverage lookup error
// is non-fatal (proceeds with a zero persona), matching the original.
func (c *leadOutreachContext) coverageGate(ctx context.Context, lead models.Lead) (models.ActorPersona, string) {
	var persona models.ActorPersona
	if c.msgType != "comment" || lead.ID <= 0 {
		return persona, ""
	}
	cov, cerr := c.db.Leads().GetLeadCoverageState(ctx, c.orgID, lead.ID, c.commentIdentity.Website)
	if cerr != nil {
		return persona, ""
	}
	if ok, reason := models.EvaluateCoverage(*cov, c.coveragePolicy, c.accountID, time.Now().UTC()); !ok {
		return persona, reason
	}
	return models.DeriveActorPersona(*cov, c.coveragePolicy, "", ""), ""
}

// prepareOutreachContent produces the per-lead content: template fallback → AI
// generation (with Knowledge OS retrieval) → trim/empty → P2c reasoning. Returns
// (content, retrievalID, skipReason, genErr).
func (c *leadOutreachContext) prepareOutreachContent(ctx context.Context, lead models.Lead, targetURL string, persona models.ActorPersona) (string, string, string, error) {
	content := c.template
	var retrievalID string
	if c.msgGen != nil && c.msgGen.Available() {
		genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		// Per-lead Knowledge OS retrieval with full trace (1.5s: LIKE searcher is fast).
		retrievalCtx, retrievalCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		var leadContext string
		leadContext, retrievalID = c.knowledgeBuilder.BuildForLeadWithTrace(retrievalCtx, c.orgID, lead.Content, c.businessContext, c.msgType+"_drafted")
		retrievalCancel()
		var genErr error
		if c.template != "" && c.msgType == "comment" {
			content, genErr = c.msgGen.GenerateCommentFromTemplate(genCtx, c.template, lead.Content, lead.Author)
		} else if c.msgType == "comment" {
			content, genErr = c.msgGen.GenerateCommentWithService(genCtx, lead.Content, lead.Author, leadContext, lead.ServiceMatch, c.commentIdentity, persona)
		} else {
			content, genErr = c.msgGen.GenerateInboxMessage(genCtx, lead.Content, lead.Author, leadContext, "")
		}
		cancel()
		if genErr != nil {
			log.Printf("[queueLeadOutreach] AI generation failed for lead %s: %v", targetURL, genErr)
			return "", retrievalID, "generation_failed", genErr
		}
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return "", retrievalID, "empty_content", nil
	}
	if c.reasoningMode != "off" && c.msgType == "comment" {
		content = applyCommentReasoning(ctx, commentReasoningInput{
			db: c.db, kb: c.knowledgeBuilder, msgGen: c.msgGen, mode: c.reasoningMode,
			profile: c.reasoningProfile, orgID: c.orgID, accountID: c.accountID,
			initiatorUserID: c.actx.InitiatorUserID, leadContent: lead.Content,
			author: lead.Author, fallback: content,
		})
	}
	return content, retrievalID, "", nil
}

package leadoutreach

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook/commenting"
)

// coverageGate applies the multi-actor coverage policy for comment leads: returns
// the derived persona and "" to proceed, or a skip reason. A coverage lookup error
// is non-fatal (proceeds with a zero persona), matching the original.
func (c *Context) coverageGate(ctx context.Context, lead models.Lead) (models.ActorPersona, string) {
	var persona models.ActorPersona
	if c.msgType != "comment" || lead.ID <= 0 {
		return persona, ""
	}
	cov, cerr := c.coverage.GetLeadCoverageState(ctx, c.orgID, lead.ID, c.commentIdentity.Website)
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
func (c *Context) prepareOutreachContent(ctx context.Context, lead models.Lead, targetURL string, persona models.ActorPersona) (string, string, string, error) {
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
		content = commenting.Apply(ctx, commenting.Input{
			Policies: c.commentPolicies, PromptLog: c.promptLog,
			KB: c.knowledgeBuilder, MsgGen: c.msgGen, Contacts: c.contacts,
			Mode: c.reasoningMode, Profile: c.reasoningProfile, OrgID: c.orgID, AccountID: c.accountID,
			InitiatorUserID: c.actx.InitiatorUserID, LeadContent: lead.Content, Author: lead.Author, Fallback: content,
		})
	}
	return content, retrievalID, "", nil
}

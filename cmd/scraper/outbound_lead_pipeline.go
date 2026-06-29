package main

import (
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/services/facebook/commenting"
	"github.com/thg/scraper/internal/services/facebook/leadoutreach"
	"github.com/thg/scraper/internal/store"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// buildLeadOutreachContext resolves the per-run config and builds the store-free
// leadoutreach.Context (cmd is the composition root: it owns the *store.Store
// resolution + the store-backed adapters; the execution spine lives in
// internal/services/facebook/leadoutreach). Side-effect order matches the original
// preamble: businessContext → reasoning profile → comment identity.
func buildLeadOutreachContext(db *store.Store, msgGen *ai.MessageGenerator, msgType string, args map[string]any, orgID, accountID int64, actx models.ActionContext) *leadoutreach.Context {
	businessContext := businessContextForOrg(db, orgID)
	knowledgeBuilder := knowledgeRuntime.NewBuilder(db.Knowledge())
	knowledgeBuilder.Recorder = db.Knowledge()
	knowledgeBuilder.TraceRec = db.Knowledge()

	contacts := fbContactDirectory{db}
	reasoningMode := commenting.Mode()
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
		commentIdentity = facebook.ResolveCommentIdentity(contacts, orgID, actx.InitiatorUserID, accountID, idProfile, nil)
	}

	// commenting.Apply runs (and originally loaded these org policies per lead) only
	// for comment runs with reasoning on — resolve once here under the same condition.
	var commentPolicies ai.CommentPolicies
	if msgType == "comment" && reasoningMode != "off" {
		commentPolicies = ai.LoadOrgCommentPolicies(db, orgID)
	}

	return leadoutreach.New(leadoutreach.Config{
		Outbound:         storeOutboundRecorder{db},
		Coverage:         storeLeadCoverage{db},
		Lifecycle:        storeLeadLifecycle{db},
		Contacts:         contacts,
		PromptLog:        storePromptLog{db},
		MsgGen:           msgGen,
		KnowledgeBuilder: knowledgeBuilder,
		MsgType:          msgType,
		OrgID:            orgID,
		AccountID:        accountID,
		Actx:             actx,
		Template:         argString(args, "template"),
		BusinessContext:  businessContext,
		ReasoningMode:    reasoningMode,
		ReasoningProfile: reasoningProfile,
		CommentIdentity:  commentIdentity,
		CommentPolicies:  commentPolicies,
		CoveragePolicy:   models.DefaultCoveragePolicy(),
	})
}

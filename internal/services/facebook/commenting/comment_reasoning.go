// Package commenting is the Facebook comment-intelligence usecase: it composes the
// AI message generator, KnowledgeOS retrieval, the policy gate, and Facebook comment
// identity into the P2c knowledge-grounded comment decision. It is FB+AI content
// logic — it deliberately lives on the Facebook service/usecase side, NOT in the
// vertical-neutral internal/outbound spine. cmd/scraper builds the adapters (store,
// msggen, ContactDirectory) and calls this usecase. (ARCHCM2b)
package commenting

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// Mode reads the P2c knowledge-reasoning switch (env, hot kill-switch — no redeploy
// to flip):
//
//	off (default) — comment generation unchanged.
//	dryrun        — compute + persist the grounded decision for observation;
//	                does NOT change the comment text.
//	live          — when the decision has a GROUNDED offer, it drives the comment
//	                text (GenerateCommentV2); knowledge_gap falls back to the
//	                existing generic generation (no regression).
//
// THG_COMMENT_REASONING_DRYRUN=1 is kept as an alias for dryrun.
func Mode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("THG_COMMENT_REASONING"))) {
	case "dryrun":
		return "dryrun"
	case "live":
		return "live"
	}
	if os.Getenv("THG_COMMENT_REASONING_DRYRUN") == "1" {
		return "dryrun"
	}
	return "off"
}

// Input groups the inputs of Apply (S107: a flat 12-arg signature). It only bundles
// existing values — no new logic or behavior. Contacts is the injected
// facebook.ContactDirectory adapter (the cmd composition root constructs it); the
// usecase never builds a concrete directory itself.
type Input struct {
	DB              *store.Store
	KB              *knowledgeRuntime.Builder
	MsgGen          *ai.MessageGenerator
	Contacts        facebook.ContactDirectory
	Mode            string
	Profile         *ai.BusinessProfile
	OrgID           int64
	AccountID       int64
	InitiatorUserID int64
	LeadContent     string
	Author          string
	Fallback        string
}

// Apply runs the Knowledge Intelligence reasoning for one comment lead. dryrun only
// observes; live lets a GROUNDED decision drive the comment text, falling back to
// Fallback on knowledge_gap or any error. The decision is persisted for observation
// in BOTH modes. Best-effort: it can never break the queue path — every failure
// returns Fallback. See specs/COMMENT_INTELLIGENCE_PIPELINE.md §9 (P2c).
func Apply(ctx context.Context, in Input) string {
	rctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	candidates, retrievalID, err := in.KB.CandidatesForLead(rctx, in.OrgID, in.LeadContent)
	if err != nil {
		log.Printf("[reasoning] candidates org=%d: %v", in.OrgID, err)
		return in.Fallback
	}
	decision, err := in.MsgGen.DecideComment(rctx, in.LeadContent, in.Author, in.Profile, candidates, retrievalID)
	if err != nil || decision == nil {
		log.Printf("[reasoning] decide org=%d: %v", in.OrgID, err)
		return in.Fallback
	}
	// P2d Policy Gate (PR-7): confidence + org policy shape what the
	// prompt may pitch — high conf → product (+price if allowed), medium
	// → category/service only (no exact price), low/gap → generic
	// fallback. Strictly subtractive over the grounded selection.
	verdict := ai.EvaluateGate(decision, ai.LoadOrgCommentPolicies(in.DB, in.OrgID))
	decision = ai.ApplyGate(decision, verdict)
	log.Printf("[reasoning:%s] org=%d account=%d intent=%s conf=%.2f knowledge_gap=%v gate=%s caps=%d products=%d proofs=%d",
		in.Mode, in.OrgID, in.AccountID, decision.Intent, decision.Confidence, decision.KnowledgeGap, verdict.Mode,
		len(decision.Selected.Capabilities), len(decision.Selected.Products), len(decision.Selected.Proofs))
	if payload, perr := json.Marshal(decision); perr == nil {
		_ = in.DB.Prompts().InsertSystemPromptLog(in.OrgID, in.AccountID,
			"agent comment decision ("+in.Mode+")", "comment_decision_"+in.Mode, string(payload), !decision.KnowledgeGap)
	}
	if in.Mode == "live" && !decision.KnowledgeGap {
		// Same resolver/contract as the normal path: staff contact channels + CTA
		// win, the company website is preserved, and the per-lead grounded CTA
		// seeds the identity. The live prompt must NOT re-derive a company-only
		// identity (that dropped the staff swap before this fix).
		liveIdentity := facebook.ResolveCommentIdentity(in.Contacts, in.OrgID, in.InitiatorUserID, in.AccountID, in.Profile, decision.Selected.CTA)
		text, gerr := in.MsgGen.GenerateCommentV2(rctx, in.LeadContent, in.Author, in.Profile, decision, liveIdentity)
		if gerr != nil {
			log.Printf("[reasoning:live] GenerateCommentV2 org=%d: %v — falling back", in.OrgID, gerr)
			return in.Fallback
		}
		if t := strings.TrimSpace(text); t != "" {
			return t // grounded decision drives the live comment text
		}
	}
	return in.Fallback
}

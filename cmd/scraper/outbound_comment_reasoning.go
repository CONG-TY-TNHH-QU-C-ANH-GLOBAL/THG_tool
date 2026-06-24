package main

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

// commentReasoningMode reads the P2c knowledge-reasoning switch (env, hot
// kill-switch — no redeploy to flip):
//
//	off (default) — comment generation unchanged.
//	dryrun        — compute + persist the grounded decision for observation;
//	                does NOT change the comment text.
//	live          — when the decision has a GROUNDED offer, it drives the comment
//	                text (GenerateCommentV2); knowledge_gap falls back to the
//	                existing generic generation (no regression).
//
// THG_COMMENT_REASONING_DRYRUN=1 is kept as an alias for dryrun.
func commentReasoningMode() string {
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

// applyCommentReasoning runs the Knowledge Intelligence reasoning for one comment
// lead. dryrun only observes; live lets a GROUNDED decision drive the comment
// text, falling back to `fallback` on knowledge_gap or any error. The decision is
// persisted for observation in BOTH modes. Best-effort: it can never break the
// queue path — every failure returns `fallback`. See
// specs/COMMENT_INTELLIGENCE_PIPELINE.md §9 (P2c).
// commentReasoningInput groups the inputs of applyCommentReasoning (S107: a flat
// 12-arg signature). It only bundles existing values — no new logic or behavior.
type commentReasoningInput struct {
	db              *store.Store
	kb              *knowledgeRuntime.Builder
	msgGen          *ai.MessageGenerator
	mode            string
	profile         *ai.BusinessProfile
	orgID           int64
	accountID       int64
	initiatorUserID int64
	leadContent     string
	author          string
	fallback        string
}

func applyCommentReasoning(ctx context.Context, in commentReasoningInput) string {
	rctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	candidates, retrievalID, err := in.kb.CandidatesForLead(rctx, in.orgID, in.leadContent)
	if err != nil {
		log.Printf("[reasoning] candidates org=%d: %v", in.orgID, err)
		return in.fallback
	}
	decision, err := in.msgGen.DecideComment(rctx, in.leadContent, in.author, in.profile, candidates, retrievalID)
	if err != nil || decision == nil {
		log.Printf("[reasoning] decide org=%d: %v", in.orgID, err)
		return in.fallback
	}
	// P2d Policy Gate (PR-7): confidence + org policy shape what the
	// prompt may pitch — high conf → product (+price if allowed), medium
	// → category/service only (no exact price), low/gap → generic
	// fallback. Strictly subtractive over the grounded selection.
	verdict := ai.EvaluateGate(decision, ai.LoadOrgCommentPolicies(in.db, in.orgID))
	decision = ai.ApplyGate(decision, verdict)
	log.Printf("[reasoning:%s] org=%d account=%d intent=%s conf=%.2f knowledge_gap=%v gate=%s caps=%d products=%d proofs=%d",
		in.mode, in.orgID, in.accountID, decision.Intent, decision.Confidence, decision.KnowledgeGap, verdict.Mode,
		len(decision.Selected.Capabilities), len(decision.Selected.Products), len(decision.Selected.Proofs))
	if payload, perr := json.Marshal(decision); perr == nil {
		_ = in.db.Prompts().InsertSystemPromptLog(in.orgID, in.accountID,
			"agent comment decision ("+in.mode+")", "comment_decision_"+in.mode, string(payload), !decision.KnowledgeGap)
	}
	if in.mode == "live" && !decision.KnowledgeGap {
		// Same resolver/contract as the normal path: staff contact channels + CTA
		// win, the company website is preserved, and the per-lead grounded CTA
		// seeds the identity. The live prompt must NOT re-derive a company-only
		// identity (that dropped the staff swap before this fix).
		liveIdentity := facebook.ResolveCommentIdentity(fbContactDirectory{in.db}, in.orgID, in.initiatorUserID, in.accountID, in.profile, decision.Selected.CTA)
		text, gerr := in.msgGen.GenerateCommentV2(rctx, in.leadContent, in.author, in.profile, decision, liveIdentity)
		if gerr != nil {
			log.Printf("[reasoning:live] GenerateCommentV2 org=%d: %v — falling back", in.orgID, gerr)
			return in.fallback
		}
		if t := strings.TrimSpace(text); t != "" {
			return t // grounded decision drives the live comment text
		}
	}
	return in.fallback
}

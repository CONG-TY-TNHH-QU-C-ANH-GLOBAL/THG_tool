package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
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

// queueLeadOutreach returns (summary, queued, err). `queued` is the number of outbound
// messages actually enqueued this run: it is 0 on every no-queue path (org guard / readiness
// block / no eligible lead / all leads skipped by coverage/dedup/policy), even when err is nil
// and `summary` carries the block/skip reason. Async callers (the direct-post scheduler) MUST
// branch on `queued == 0` so a no-op is never recorded as a queued/completed comment.
func queueLeadOutreach(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, msgType string, args map[string]any, notify func(string)) (string, int, error) {
	orgID := argInt64(args, "org_id")
	if orgID <= 0 {
		return "", 0, fmt.Errorf("org_id is required for outbound automation")
	}
	userID := argInt64(args, "user_id")
	role := argString(args, "user_role")
	// Resolve the campaign-ready ActionContext (Source=manual). The queue path
	// below consumes only the context, so a future ResolveCampaignActionContext
	// drops in without touching this code.
	actx, err := resolveUserActionContext(db, orgID, userID, role, argInt64(args, "account_id"), true)
	if err != nil {
		return "", 0, err
	}
	accountID := actx.AccountID

	// §5 readiness preflight: block a comment run up-front when the resolved
	// Facebook account cannot execute instead of queueing comments that imply
	// posting but can never run. Comment-only here; inbox keeps its behavior.
	if msgType == "comment" {
		if blockMsg, blocked := facebook.CommentReadinessGate(ctx, fbCommentReadinessEvaluator{db}, orgID, userID, role, accountID); blocked {
			return blockMsg, 0, nil
		}
	}

	leads, err := facebook.LeadsForAction(ctx, fbLeadSource{db}, orgID, msgType, leadSelectionInputFromArgs(args))
	if err != nil {
		return "", 0, err
	}
	if len(leads) == 0 {
		return "khong co lead phu hop de queue outbound", 0, nil
	}

	// requestedAuto carries the AI/agent's preference. The store layer
	// (QueueOutboundForOrg -> IsAutoOutboundEnabledForOrg) is the final
	// gatekeeper: it downgrades to draft if the org has not opted in.
	requestedAuto := argBool(args, "auto")
	run := buildLeadOutreachContext(db, msgGen, msgType, args, orgID, accountID, actx)
	st := newLeadOutreachState()
	// Eligible-fill (PR-2): keep scanning the candidate pool past skipped leads
	// until `requested` are queued or the pool is exhausted.
	requested := facebook.RequestedOutreachCount(int(argInt64(args, "limit")), int(argInt64(args, "max_items")))
	for _, lead := range leads {
		if st.queued >= requested {
			break
		}
		st.scanned++
		if err := run.processOutreachLead(ctx, lead, st); err != nil {
			return "", st.queued, err
		}
	}
	return run.formatOutreachResult(ctx, requestedAuto, notify, st), st.queued, nil
}

func queueGroupPost(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, notify func(string)) (string, error) {
	targets := []string{}
	if u := argString(args, "group_url"); u != "" {
		targets = append(targets, u)
	} else {
		orgID := argInt64(args, "org_id")
		groups, err := db.Crawl().GetAllGroups(orgID)
		if err != nil {
			return "", err
		}
		for _, g := range groups {
			if g.Active && strings.TrimSpace(g.URL) != "" {
				targets = append(targets, g.URL)
				if len(targets) >= 3 {
					break
				}
			}
		}
	}
	if len(targets) == 0 {
		return "khong co group target de queue group_post", nil
	}
	return queueFacebookPostTargets(ctx, db, msgGen, args, "group_post", targets, notify)
}

func queueProfilePost(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	userID := argInt64(args, "user_id")
	role := argString(args, "user_role")
	accountID, err := resolveCallerAccountID(db, orgID, userID, role, argInt64(args, "account_id"), false)
	if err != nil {
		return "", err
	}
	// Persist the resolved+owner-checked account_id so queueFacebookPostTargets
	// uses it instead of resolving again.
	args["account_id"] = accountID
	target, skipReason := facebook.ResolveProfilePostTarget(db.Identities().GetAccountForOrg, orgID, accountID, argString(args, "profile_url"))
	if skipReason != "" {
		return fmt.Sprintf("queued_profile_post=0 skipped=1 mode=skipped reasons=map[%s:1]", skipReason), nil
	}
	return queueFacebookPostTargets(ctx, db, msgGen, args, "profile_post", []string{target}, notify)
}

func queueFacebookPostTargets(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, msgType string, targets []string, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	if orgID <= 0 {
		return "", fmt.Errorf("org_id is required for Facebook posting")
	}
	userID := argInt64(args, "user_id")
	role := argString(args, "user_role")
	accountID, err := resolveCallerAccountID(db, orgID, userID, role, argInt64(args, "account_id"), false)
	if err != nil {
		return "", err
	}

	content, err := facebook.ResolveFacebookPostContent(ctx, msgGen, postContentInputFromArgs(args))
	if err != nil {
		return "", err
	}

	requestedAuto := argBool(args, "auto")
	queued, skipped := 0, 0
	approvedCount := 0
	for _, target := range targets {
		result, err := db.QueueOutboundForOrg(&models.OutboundMessage{
			OrgID:     orgID,
			Type:      msgType,
			Platform:  models.PlatformFacebook,
			AccountID: accountID,
			TargetURL: target,
			Content:   strings.TrimSpace(content),
			AIModel:   "agent",
			CreatedBy: userID,
		}, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if !result.Decision.Allowed {
			skipped++
			continue
		}
		queued++
		if result.ExecutionState == models.ExecPlanned {
			approvedCount++
		}
	}
	mode := outreachMode(approvedCount, queued, requestedAuto)
	if notify != nil && queued > 0 {
		notify(facebook.FormatOutboundNotification(orgID, accountID, msgType, queued, skipped, mode))
	}
	return fmt.Sprintf("queued_%s=%d skipped=%d mode=%s", msgType, queued, skipped, mode), nil
}

// resolveCallerAccountID picks the FB account_id the skill executor will use,
// enforcing execution-layer ownership per RBAC-1 (see
// feedback_shared_battlefield_not_crm.md):
//
//   - If requestedAccountID > 0: load it and verify the caller owns it.
//     Admin / platform roles pass; sales must match acc.AssignedUserID.
//   - If requestedAccountID <= 0 and the caller is identified (userID > 0):
//     pick from the caller's OWNED accounts only (sales = GetAccountsForUser,
//     admin / platform = GetAllAccounts).
//   - If userID <= 0 (Telegram bot / legacy unauthenticated path): pick
//     from any account in the org (preserves current behaviour; future PR
//     resolves Telegram operator → DB user).
//
// preferLoggedIn rewards the first FB-platform, browser-logged-in, active
// account in the candidate list (legacy lead-outreach behaviour). Set to
// false for post / profile_post paths that don't need a logged-in browser.
// resolveUserActionContext produces the campaign-ready models.ActionContext for
// a member-initiated (Source=manual) outbound. It wraps the deterministic
// account resolution; a future resolveCampaignActionContext returns the SAME
// shape so the execution path stays source-agnostic (campaign is additive).
// ConnectorID/CampaignID/ExecutionSourceID are left 0 — filled by the future
// connector-availability + campaign layers.
func resolveUserActionContext(db *store.Store, orgID, userID int64, role string, requestedAccountID int64, preferLoggedIn bool) (models.ActionContext, error) {
	accID, err := resolveCallerAccountID(db, orgID, userID, role, requestedAccountID, preferLoggedIn)
	if err != nil {
		return models.ActionContext{}, err
	}
	return models.ActionContext{
		OrgID:           orgID,
		Source:          models.ActionSourceManual,
		InitiatorUserID: userID,
		AccountID:       accID,
	}, nil
}

func resolveCallerAccountID(db *store.Store, orgID, userID int64, role string, requestedAccountID int64, preferLoggedIn bool) (int64, error) {
	if requestedAccountID > 0 {
		acc, err := db.Identities().GetAccountForOrg(requestedAccountID, orgID)
		if err != nil || acc == nil {
			return 0, fmt.Errorf("account_id %d not found in org %d", requestedAccountID, orgID)
		}
		if userID > 0 && !models.IsAccountOwnerAllowed(acc, userID, role) {
			return 0, fmt.Errorf("you do not own account #%d", requestedAccountID)
		}
		return acc.ID, nil
	}

	var candidates []models.Account
	var err error
	if userID > 0 {
		r := models.UserRole(strings.ToLower(strings.TrimSpace(role)))
		if models.IsPlatformRole(r) || r == models.RoleAdmin {
			candidates, err = db.Identities().GetAllAccounts(orgID)
		} else {
			candidates, err = db.Identities().GetAccountsForUser(orgID, userID)
		}
	} else {
		// Legacy / unauthenticated path: any org account.
		candidates, err = db.Identities().GetAllAccounts(orgID)
	}
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		if userID > 0 {
			return 0, fmt.Errorf("you have no Facebook account assigned in org %d; ask an admin to assign one", orgID)
		}
		return 0, fmt.Errorf("no Facebook account available for org %d", orgID)
	}
	if preferLoggedIn {
		// Deterministic ExecutionContext (Organic Sales Network): NO heuristic,
		// NO "first logged-in", NO newest-connector, NO auto-magic default.
		// Resolution order: explicit account_id (handled above) -> user Default
		// Account -> exactly-one-owned-account -> error execution_context_required.
		ownedIDs := make(map[int64]bool, len(candidates))
		for _, acc := range candidates {
			ownedIDs[acc.ID] = true
		}
		if def := db.GetUserDefaultAccount(orgID, userID); def > 0 && ownedIDs[def] {
			return def, nil
		}
		var usable []int64
		for _, acc := range candidates {
			if acc.Platform == models.PlatformFacebook && acc.Status == models.AccountActive {
				usable = append(usable, acc.ID)
			}
		}
		if len(usable) == 1 {
			return usable[0], nil
		}
		if len(usable) == 0 {
			return 0, fmt.Errorf("execution_context_required: no usable Facebook account — pair a Chrome connector and log into Facebook first")
		}
		return 0, fmt.Errorf("execution_context_required: you have %d Facebook accounts — set a Default Account in Settings (or pass account_id)", len(usable))
	}
	return candidates[0].ID, nil
}

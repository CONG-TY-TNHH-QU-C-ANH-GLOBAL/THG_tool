package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
)

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

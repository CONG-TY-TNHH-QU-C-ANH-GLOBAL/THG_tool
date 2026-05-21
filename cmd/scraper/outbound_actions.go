package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

func queueLeadOutreach(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, msgType string, args map[string]any, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	if orgID <= 0 {
		return "", fmt.Errorf("org_id is required for outbound automation")
	}
	userID := argInt64(args, "user_id")
	role := argString(args, "user_role")
	accountID, err := resolveCallerAccountID(db, orgID, userID, role, argInt64(args, "account_id"), true)
	if err != nil {
		return "", err
	}

	// requestedAuto carries the AI/agent's preference. The store layer
	// (QueueOutboundForOrg -> IsAutoOutboundEnabledForOrg) is the final
	// gatekeeper: it downgrades to draft if the org has not opted in.
	requestedAuto := argBool(args, "auto")

	leads, err := leadsFromActionArgs(db, orgID, msgType, args)
	if err != nil {
		return "", err
	}
	if len(leads) == 0 {
		return "khong co lead phu hop de queue outbound", nil
	}

	businessContext := businessContextForOrg(db, orgID)
	// Knowledge OS runtime builder. Per-lead retrieval augments the
	// org-wide freeform business profile with top-K matched assets
	// (PRODUCTS, POLICIES, CTAs). When the org has not configured a
	// Knowledge OS source, BuildForLeadWithTrace returns businessContext
	// unchanged — backward compatible by construction. See
	// specs/WORKSPACE_KNOWLEDGE_OS.md §11 (Migration path).
	//
	// TraceRec wires the Operator Replay surface: each retrieval gets
	// a full Trace + Budget recorded under a retrievalID we thread into
	// the outcome event when the message is queued. Replay UI joins on
	// retrievalID to show "this lead → these assets → this outcome".
	knowledgeBuilder := knowledgeRuntime.NewBuilder(db.Knowledge())
	knowledgeBuilder.Recorder = db.Knowledge()
	knowledgeBuilder.TraceRec = db.Knowledge()
	template := argString(args, "template")
	queued, skipped := 0, 0
	approvedCount := 0
	skipReasons := map[string]int{}
	var lastGenErr error
	for _, lead := range leads {
		targetURL := strings.TrimSpace(lead.SourceURL)
		profileURL := strings.TrimSpace(lead.AuthorURL)
		if msgType == "inbox" {
			targetURL = profileURL
		}
		if targetURL == "" {
			skipped++
			skipReasons["missing_target"]++
			continue
		}
		if msgType == "comment" && !isCommentableFacebookPostURL(targetURL) {
			skipped++
			skipReasons["missing_post_permalink"]++
			continue
		}

		content := template
		var retrievalID string
		if msgGen != nil && msgGen.Available() {
			genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			// Per-lead Knowledge OS retrieval with full trace.
			// 1.5s timeout because the LIKE-based naive searcher is fast;
			// pgvector replacement should still fit comfortably.
			retrievalCtx, retrievalCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			generatedAction := msgType + "_drafted"
			var leadContext string
			leadContext, retrievalID = knowledgeBuilder.BuildForLeadWithTrace(retrievalCtx, orgID, lead.Content, businessContext, generatedAction)
			retrievalCancel()
			var genErr error
			if template != "" && msgType == "comment" {
				content, genErr = msgGen.GenerateCommentFromTemplate(genCtx, template, lead.Content, lead.Author)
			} else if msgType == "comment" {
				content, genErr = msgGen.GenerateCommentWithService(genCtx, lead.Content, lead.Author, leadContext, lead.ServiceMatch, "")
			} else {
				content, genErr = msgGen.GenerateInboxMessage(genCtx, lead.Content, lead.Author, leadContext, "")
			}
			cancel()
			if genErr != nil {
				log.Printf("[queueLeadOutreach] AI generation failed for lead %s: %v", targetURL, genErr)
				lastGenErr = genErr
				skipped++
				skipReasons["generation_failed"]++
				continue
			}
		}
		content = strings.TrimSpace(content)
		if content == "" {
			skipped++
			skipReasons["empty_content"]++
			continue
		}

		result, err := db.QueueOutboundForOrg(&models.OutboundMessage{
			OrgID:      orgID,
			Type:       msgType,
			Platform:   models.PlatformFacebook,
			AccountID:  accountID,
			TargetURL:  targetURL,
			TargetName: lead.Author,
			Content:    content,
			Context:    lead.Content,
			AIModel:    "agent",
		}, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if !result.Decision.Allowed {
			skipped++
			skipReasons[result.Decision.Reason]++
			// Record the rejection outcome so the Operator Replay UI
			// shows "retrieved → drafted → rejected (reason)" instead
			// of leaving the retrieval event dangling.
			if retrievalID != "" {
				db.Knowledge().RecordOutcome(ctx, orgID, retrievalID, "rejected")
			}
			continue
		}
		queued++
		if result.ExecutionState == models.ExecPlanned {
			approvedCount++
		}
		// Stage outcome: queue success. The downstream browser-execution
		// layer is responsible for the FINAL "sent" / "failed" outcome
		// against the same retrievalID — that's where image attachments
		// (Phase E) and DOM verification land.
		if retrievalID != "" {
			db.Knowledge().RecordOutcome(ctx, orgID, retrievalID, "queued")
		}
	}

	mode := "draft"
	switch {
	case approvedCount > 0 && approvedCount == queued:
		mode = "approved_auto"
	case approvedCount > 0:
		mode = "mixed"
	case requestedAuto:
		// Caller asked for auto but the org is not opted in; make this
		// visible in the response so the operator knows why it queued as draft.
		mode = "draft_org_not_auto"
	}
	if notify != nil && queued > 0 {
		notify(formatOutboundNotification(orgID, accountID, msgType, queued, skipped, mode))
	}

	errDetails := ""
	if lastGenErr != nil {
		errDetails = fmt.Sprintf(" | Last Error: %v", lastGenErr)
	}

	return fmt.Sprintf("queued_%s=%d skipped=%d mode=%s reasons=%v%s", msgType, queued, skipped, mode, skipReasons, errDetails), nil
}

func leadsFromActionArgs(db *store.Store, orgID int64, msgType string, args map[string]any) ([]models.Lead, error) {
	if msgType == "comment" {
		if target := textutil.FirstNonEmpty(argString(args, "post_url"), argString(args, "target_url")); target != "" {
			return []models.Lead{{
				OrgID:      orgID,
				SourceURL:  target,
				Author:     argString(args, "target_name"),
				AuthorURL:  argString(args, "author_url"),
				Content:    argString(args, "context"),
				Score:      models.LeadHot,
				Platform:   models.PlatformFacebook,
				SourceType: "prompt_target",
			}}, nil
		}
	} else if target := argString(args, "target_url"); target != "" {
		return []models.Lead{{
			OrgID:      orgID,
			AuthorURL:  target,
			Author:     argString(args, "target_name"),
			Content:    argString(args, "context"),
			Score:      models.LeadHot,
			Platform:   models.PlatformFacebook,
			SourceType: "prompt_target",
		}}, nil
	}
	score := argString(args, "score_filter")
	if score == "" && msgType == "inbox" {
		score = "hot"
	}
	limit := int(argInt64(args, "limit"))
	if limit <= 0 {
		limit = 25
	}
	return db.GetAutomationLeadsForOrg(orgID, score, limit)
}

func isCommentableFacebookPostURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	if host != "fb.watch" && !strings.HasSuffix(host, ".fb.watch") &&
		host != "facebook.com" && !strings.HasSuffix(host, ".facebook.com") {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(u.EscapedPath()))
	if (host == "fb.watch" || strings.HasSuffix(host, ".fb.watch")) && strings.Trim(path, "/") != "" {
		return true
	}
	query := u.Query()
	if query.Get("story_fbid") != "" || query.Get("multi_permalinks") != "" {
		return true
	}
	if strings.Contains(path, "/posts/") ||
		strings.Contains(path, "/permalink/") ||
		strings.Contains(path, "/videos/") ||
		strings.Contains(path, "/reel/") ||
		strings.Contains(path, "/watch/") ||
		strings.Contains(path, "/share/") {
		return true
	}
	if strings.HasSuffix(path, "/photo.php") && query.Get("fbid") != "" {
		return true
	}
	return false
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
	target := argString(args, "profile_url")
	if target == "" && accountID > 0 {
		if acc, err := db.GetAccountForOrg(accountID, orgID); err == nil && acc != nil {
			target = textutil.FirstNonEmpty(acc.FBProfileURL, "https://www.facebook.com/me")
		}
	}
	if target == "" {
		target = "https://www.facebook.com/me"
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

	content := textutil.FirstNonEmpty(argString(args, "content"), argString(args, "description"), argString(args, "title"))
	if msgGen != nil && msgGen.Available() && argString(args, "title") != "" {
		genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		generated, err := msgGen.GenerateJobPost(genCtx,
			argString(args, "title"),
			argString(args, "description"),
			argString(args, "requirements"),
			argString(args, "benefits"),
			argString(args, "salary"),
			argString(args, "email"),
		)
		cancel()
		if err == nil && strings.TrimSpace(generated) != "" {
			content = generated
		}
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("Facebook post content is required")
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
	mode := "draft"
	switch {
	case approvedCount > 0 && approvedCount == queued:
		mode = "approved_auto"
	case approvedCount > 0:
		mode = "mixed"
	case requestedAuto:
		mode = "draft_org_not_auto"
	}
	if notify != nil && queued > 0 {
		notify(formatOutboundNotification(orgID, accountID, msgType, queued, skipped, mode))
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
func resolveCallerAccountID(db *store.Store, orgID, userID int64, role string, requestedAccountID int64, preferLoggedIn bool) (int64, error) {
	if requestedAccountID > 0 {
		acc, err := db.GetAccountForOrg(requestedAccountID, orgID)
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
			candidates, err = db.GetAllAccounts(orgID)
		} else {
			candidates, err = db.GetAccountsForUser(orgID, userID)
		}
	} else {
		// Legacy / unauthenticated path: any org account.
		candidates, err = db.GetAllAccounts(orgID)
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
		for _, acc := range candidates {
			if acc.Platform == models.PlatformFacebook && acc.BrowserLoggedIn && acc.Status == models.AccountActive {
				return acc.ID, nil
			}
		}
	}
	return candidates[0].ID, nil
}

func formatOutboundNotification(orgID, accountID int64, msgType string, queued, skipped int, mode string) string {
	label := "outbound"
	switch msgType {
	case "comment":
		label = "Facebook comments"
	case "inbox":
		label = "Facebook inbox"
	case "group_post":
		label = "Facebook posting"
	case "profile_post":
		label = "Facebook profile posting"
	}
	state := "drafts waiting for approval"
	if mode == "approved_auto" {
		state = "approved for Chrome Extension execution"
	}
	return fmt.Sprintf("[THG Agent] %s queued: %d (%s). Org #%d, account #%d, skipped %d by guardrails.", label, queued, state, orgID, accountID, skipped)
}

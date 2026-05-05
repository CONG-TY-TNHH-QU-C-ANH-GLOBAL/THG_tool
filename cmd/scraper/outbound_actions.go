package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

func queueLeadOutreach(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, msgType string, args map[string]any, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	if orgID <= 0 {
		return "", fmt.Errorf("org_id is required for outbound automation")
	}
	accountID := argInt64(args, "account_id")
	if accountID <= 0 {
		accounts, err := db.GetAllAccounts(orgID)
		if err != nil {
			return "", err
		}
		for _, acc := range accounts {
			if acc.Platform == models.PlatformFacebook && acc.BrowserLoggedIn && acc.Status == models.AccountActive {
				accountID = acc.ID
				break
			}
		}
		if accountID <= 0 && len(accounts) > 0 {
			accountID = accounts[0].ID
		}
	}
	if accountID <= 0 {
		return "", fmt.Errorf("no Facebook account available for org %d", orgID)
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
	template := argString(args, "template")
	queued, skipped := 0, 0
	approvedCount := 0
	skipReasons := map[string]int{}
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

		content := template
		if msgGen != nil && msgGen.Available() {
			genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			var genErr error
			if template != "" && msgType == "comment" {
				content, genErr = msgGen.GenerateCommentFromTemplate(genCtx, template, lead.Content, lead.Author)
			} else if msgType == "comment" {
				content, genErr = msgGen.GenerateCommentWithService(genCtx, lead.Content, lead.Author, businessContext, lead.ServiceMatch, "")
			} else {
				content, genErr = msgGen.GenerateInboxMessage(genCtx, lead.Content, lead.Author, businessContext, "")
			}
			cancel()
			if genErr != nil {
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
		}, requestedAuto, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if !result.Decision.Allowed {
			skipped++
			skipReasons[result.Decision.Reason]++
			continue
		}
		queued++
		if result.Status == models.OutboundApproved {
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
		// Caller asked for auto but the org is not opted in; make this
		// visible in the response so the operator knows why it queued as draft.
		mode = "draft_org_not_auto"
	}
	if notify != nil && queued > 0 {
		notify(formatOutboundNotification(orgID, accountID, msgType, queued, skipped, mode))
	}
	return fmt.Sprintf("queued_%s=%d skipped=%d mode=%s reasons=%v", msgType, queued, skipped, mode, skipReasons), nil
}

func leadsFromActionArgs(db *store.Store, orgID int64, msgType string, args map[string]any) ([]models.Lead, error) {
	if msgType == "comment" {
		if target := firstNonEmpty(argString(args, "post_url"), argString(args, "target_url")); target != "" {
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

func queueGroupPost(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, notify func(string)) (string, error) {
	targets := []string{}
	if u := argString(args, "group_url"); u != "" {
		targets = append(targets, u)
	} else {
		orgID := argInt64(args, "org_id")
		groups, err := db.GetAllGroups(orgID)
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
	accountID := argInt64(args, "account_id")
	target := argString(args, "profile_url")
	if target == "" && accountID > 0 {
		if acc, err := db.GetAccountForOrg(accountID, orgID); err == nil && acc != nil {
			target = firstNonEmpty(acc.FBProfileURL, "https://www.facebook.com/me")
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
	accountID := argInt64(args, "account_id")
	if accountID <= 0 {
		accounts, err := db.GetAllAccounts(orgID)
		if err != nil {
			return "", err
		}
		if len(accounts) == 0 {
			return "", fmt.Errorf("no Facebook account available for org %d", orgID)
		}
		accountID = accounts[0].ID
	}

	content := firstNonEmpty(argString(args, "content"), argString(args, "description"), argString(args, "title"))
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
		}, requestedAuto, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if !result.Decision.Allowed {
			skipped++
			continue
		}
		queued++
		if result.Status == models.OutboundApproved {
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

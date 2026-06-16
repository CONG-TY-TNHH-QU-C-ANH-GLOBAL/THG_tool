package main

import (
	"context"
	"fmt"
	"net/url"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

func makeAgentActionHandler(db *store.Store, jobStore *jobs.Store, msgGen *ai.MessageGenerator, notify func(string)) func(string, map[string]any) (string, error) {
	// Direct-post intake service (P1 PR-2): unknown direct-comment posts are imported
	// + continued durably instead of asking the user to manually scan/import.
	intake := newDirectPostIntake(db, jobStore)
	return func(action string, args map[string]any) (string, error) {
		switch action {
		case "set_context":
			key, value := argString(args, "key"), argString(args, "value")
			if key == "" || value == "" {
				return "", fmt.Errorf("set_context requires key and value")
			}
			// Approval policy keys (outbound_mode, auto_comment_mode) are
			// admin-controlled. AI tools must NOT be able to flip the org
			// into auto-execute via prompt — that is exactly the
			// prompt-injection vector flagged in the 2026-05-03 audit.
			// Operators set outbound_mode via the dashboard / admin API.
			switch key {
			case "outbound_mode", "auto_comment_mode", "org:outbound_mode":
				return "", fmt.Errorf("outbound_mode is admin-controlled; ask the workspace owner to change it in Dashboard › Settings, not via AI prompt")
			}
			if orgID := argInt64(args, "org_id"); orgID > 0 {
				switch key {
				case "business_profile", "private_files_summary", "data_sources_summary":
					key = fmt.Sprintf("org:%d:%s", orgID, key)
				}
			}
			if err := db.Leads().SetContext(key, value); err != nil {
				return "", err
			}
			return fmt.Sprintf("da luu context %q", key), nil
		case "describe_business":
			desc := argString(args, "description")
			if desc == "" {
				return "", fmt.Errorf("describe_business requires description")
			}
			key := "business_desc"
			if orgID := argInt64(args, "org_id"); orgID > 0 {
				key = fmt.Sprintf("org:%d:business_profile", orgID)
			}
			if err := db.Leads().SetContext(key, desc); err != nil {
				return "", err
			}
			return "da luu mo ta doanh nghiep cho crawler/classifier", nil
		case "get_stats":
			stats, err := db.App().GetStats()
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("posts=%d leads=%d hot=%d jobs_running=%d", stats.TotalPosts, stats.TotalLeads, stats.HotLeads, stats.RunningJobs), nil
		case "add_group":
			u, name := argString(args, "url"), argString(args, "name")
			if u == "" {
				return "", fmt.Errorf("add_group requires url")
			}
			if name == "" {
				name = u
			}
			id, err := db.Crawl().AddGroup(&models.Group{
				OrgID:     argInt64(args, "org_id"),
				Platform:  detectPlatformFromURL(u),
				Name:      name,
				URL:       u,
				Active:    true,
				JoinState: "none",
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("da them group #%d", id), nil
		case "scrape_group":
			u := argString(args, "url")
			if u == "" {
				return "", fmt.Errorf("scrape_group requires url")
			}
			return submitOpenCrawl(context.Background(), db, jobStore, "facebook_crawl", []jobs.Source{{Type: sourceTypeFromURL(u), URL: u, Label: "prompt_url"}}, args)
		case "scrape_comments":
			u := argString(args, "post_url")
			if u == "" {
				return "", fmt.Errorf("scrape_comments requires post_url")
			}
			return submitOpenCrawl(context.Background(), db, jobStore, "facebook_crawl", []jobs.Source{{Type: "facebook_post", URL: u, Label: "prompt_post"}}, args)
		case "classify_leads":
			return "classification runs inline during every crawler job using the current business context", nil
		case "search_groups":
			query := argString(args, "query")
			if query == "" {
				return "", fmt.Errorf("search_groups requires query")
			}
			searchURL := "https://www.facebook.com/search/groups/?q=" + url.QueryEscape(query)
			return submitOpenCrawl(context.Background(), db, jobStore, "facebook_crawl", []jobs.Source{{Type: "facebook_search", URL: searchURL, Label: "group_search"}}, args)
		case "auto_comment", "comment_all_leads":
			// PR-2 DISTRIBUTED pool: run on the requester-controllable live accounts
			// (live ∩ owned). Other members' live accounts are silently excluded — never
			// enlisted, never a reason to fail the whole action. Empty pool fails closed.
			return runPooledOutreach(context.Background(), db, msgGen, "comment", args, notify)
		case "comment_single_post":
			// comment_single_post runs guardFacebookWriteAccount inside commentSinglePost
			// (after URL resolution), so it is not double-guarded here.
			return commentSinglePost(context.Background(), db, msgGen, args, notify, intake)
		case "auto_inbox", "inbox_all_leads":
			// PR-2 DISTRIBUTED pool: same ownership-filtered pool rule as comment.
			return runPooledOutreach(context.Background(), db, msgGen, "inbox", args, notify)
		case "create_job_post":
			if msg, blocked := guardFacebookWriteAccount(db, args); blocked {
				return msg, nil
			}
			return queueGroupPost(context.Background(), db, msgGen, args, notify)
		case "post_to_profile":
			if msg, blocked := guardFacebookWriteAccount(db, args); blocked {
				return msg, nil
			}
			return queueProfilePost(context.Background(), db, msgGen, args, notify)
		default:
			return "", fmt.Errorf("agent action %q is not wired to a production handler yet", action)
		}
	}
}

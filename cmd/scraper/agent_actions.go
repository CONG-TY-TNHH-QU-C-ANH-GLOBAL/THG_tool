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

// agentActionRouter carries the dependencies shared by every agent-action
// handler. Extracting the per-action logic into methods keeps the dispatcher
// (handle) a thin switch instead of one deeply-nested closure — the prior
// shape tripped go:S3776 (cognitive complexity 55). Behavior is unchanged:
// each method holds the exact body its switch case used to inline.
type agentActionRouter struct {
	db       *store.Store
	jobStore *jobs.Store
	msgGen   *ai.MessageGenerator
	notify   func(string)
	intake   *directPostIntake
}

func makeAgentActionHandler(db *store.Store, jobStore *jobs.Store, msgGen *ai.MessageGenerator, notify func(string)) func(string, map[string]any) (string, error) {
	// Direct-post intake service (P1 PR-2): unknown direct-comment posts are imported
	// + continued durably instead of asking the user to manually scan/import.
	r := &agentActionRouter{
		db:       db,
		jobStore: jobStore,
		msgGen:   msgGen,
		notify:   notify,
		intake:   newDirectPostIntake(db, jobStore),
	}
	return r.handle
}

// handle dispatches a normalized agent action to its handler. It owns routing
// only — each case is a single call so the nesting (and cognitive complexity)
// lives in the small, independently-readable methods below.
func (r *agentActionRouter) handle(action string, args map[string]any) (string, error) {
	switch action {
	case "set_context":
		return r.setContext(args)
	case "describe_business":
		return r.describeBusiness(args)
	case "get_stats":
		return r.getStats()
	case "add_group":
		return r.addGroup(args)
	case "scrape_group":
		return r.scrapeGroup(args)
	case "scrape_comments":
		return r.scrapeComments(args)
	case "classify_leads":
		return "classification runs inline during every crawler job using the current business context", nil
	case "search_groups":
		return r.searchGroups(args)
	case "auto_comment", "comment_all_leads":
		// PR-2 DISTRIBUTED pool: run on the requester-controllable live accounts
		// (live ∩ owned). Other members' live accounts are silently excluded — never
		// enlisted, never a reason to fail the whole action. Empty pool fails closed.
		return runPooledOutreach(context.Background(), r.db, r.msgGen, "comment", args, r.notify)
	case "comment_single_post":
		// comment_single_post runs guardFacebookWriteAccount inside commentSinglePost
		// (after URL resolution), so it is not double-guarded here.
		return commentSinglePost(context.Background(), r.db, r.msgGen, args, r.notify, r.intake)
	case "auto_inbox", "inbox_all_leads":
		// PR-2 DISTRIBUTED pool: same ownership-filtered pool rule as comment.
		return runPooledOutreach(context.Background(), r.db, r.msgGen, "inbox", args, r.notify)
	case "create_job_post":
		return r.createJobPost(args)
	case "post_to_profile":
		return r.postToProfile(args)
	default:
		return "", fmt.Errorf("agent action %q is not wired to a production handler yet", action)
	}
}

func (r *agentActionRouter) setContext(args map[string]any) (string, error) {
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
	if err := r.db.Leads().SetContext(key, value); err != nil {
		return "", err
	}
	return fmt.Sprintf("da luu context %q", key), nil
}

func (r *agentActionRouter) describeBusiness(args map[string]any) (string, error) {
	desc := argString(args, "description")
	if desc == "" {
		return "", fmt.Errorf("describe_business requires description")
	}
	key := "business_desc"
	if orgID := argInt64(args, "org_id"); orgID > 0 {
		key = fmt.Sprintf("org:%d:business_profile", orgID)
	}
	if err := r.db.Leads().SetContext(key, desc); err != nil {
		return "", err
	}
	return "da luu mo ta doanh nghiep cho crawler/classifier", nil
}

func (r *agentActionRouter) getStats() (string, error) {
	stats, err := r.db.App().GetStats()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("posts=%d leads=%d hot=%d jobs_running=%d", stats.TotalPosts, stats.TotalLeads, stats.HotLeads, stats.RunningJobs), nil
}

func (r *agentActionRouter) addGroup(args map[string]any) (string, error) {
	u, name := argString(args, "url"), argString(args, "name")
	if u == "" {
		return "", fmt.Errorf("add_group requires url")
	}
	if name == "" {
		name = u
	}
	id, err := r.db.Crawl().AddGroup(&models.Group{
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
}

func (r *agentActionRouter) scrapeGroup(args map[string]any) (string, error) {
	u := argString(args, "url")
	if u == "" {
		return "", fmt.Errorf("scrape_group requires url")
	}
	return submitOpenCrawl(context.Background(), r.db, r.jobStore, "facebook_crawl", []jobs.Source{{Type: sourceTypeFromURL(u), URL: u, Label: "prompt_url"}}, args)
}

func (r *agentActionRouter) scrapeComments(args map[string]any) (string, error) {
	u := argString(args, "post_url")
	if u == "" {
		return "", fmt.Errorf("scrape_comments requires post_url")
	}
	return submitOpenCrawl(context.Background(), r.db, r.jobStore, "facebook_crawl", []jobs.Source{{Type: "facebook_post", URL: u, Label: "prompt_post"}}, args)
}

func (r *agentActionRouter) searchGroups(args map[string]any) (string, error) {
	query := argString(args, "query")
	if query == "" {
		return "", fmt.Errorf("search_groups requires query")
	}
	searchURL := "https://www.facebook.com/search/groups/?q=" + url.QueryEscape(query)
	return submitOpenCrawl(context.Background(), r.db, r.jobStore, "facebook_crawl", []jobs.Source{{Type: "facebook_search", URL: searchURL, Label: "group_search"}}, args)
}

func (r *agentActionRouter) createJobPost(args map[string]any) (string, error) {
	if msg, blocked := guardFacebookWriteAccount(r.db, args); blocked {
		return msg, nil
	}
	return queueGroupPost(context.Background(), r.db, r.msgGen, args, r.notify)
}

func (r *agentActionRouter) postToProfile(args map[string]any) (string, error) {
	if msg, blocked := guardFacebookWriteAccount(r.db, args); blocked {
		return msg, nil
	}
	return queueProfilePost(context.Background(), r.db, r.msgGen, args, r.notify)
}

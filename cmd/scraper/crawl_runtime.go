package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/crawler"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
)

// submitOpenCrawl is the thin cmd facade: it resolves the raw args bag into a typed
// crawlRequest (the arg/prompt plumbing, ARCHCM4a) and delegates to the typed core.
// Callers are unchanged; behavior is identical.
// submitOpenCrawl is the cmd composition-root facade: resolve the raw args bag into a
// typed crawler.CrawlRequest (arg/prompt facade + RBAC account auto-pick, both kept in
// cmd) and delegate to the crawler execution core (ARCHCM4b). Behavior is unchanged.
func submitOpenCrawl(ctx context.Context, db *store.Store, jobStore *jobs.Store, intent string, sources []jobs.Source, args map[string]any) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("crawler requires at least one source")
	}
	return crawler.SubmitCrawlRequest(ctx, db, jobStore, resolveCrawlRequest(db, intent, sources, args))
}

// resolveCrawlMaxItems applies the max_items → limit → prompt → default(50)
// fallback chain used for every open crawl.
func resolveCrawlMaxItems(args map[string]any) int {
	maxItems := int(argInt64(args, "max_items"))
	if maxItems <= 0 {
		maxItems = int(argInt64(args, "limit"))
	}
	if maxItems <= 0 {
		maxItems = maxItemsFromPrompt(argString(args, "user_prompt"))
	}
	if maxItems <= 0 {
		maxItems = 50
	}
	return maxItems
}

// resolveCrawlKeywords reads explicit keywords, falling back to keywords
// inferred from the user prompt when none are supplied.
func resolveCrawlKeywords(args map[string]any) []string {
	keywords := splitKeywords(argString(args, "keywords"))
	if len(keywords) == 0 {
		keywords = splitKeywords(promptKeywordFallback(argString(args, "user_prompt")))
	}
	return keywords
}

// resolveCrawlAccountID returns the explicit account_id when present; otherwise,
// for an org-scoped request with a DB, it auto-picks a ready Facebook account
// (member-ownership filtered) and writes the choice back into args["account_id"]
// so downstream routing sees the same account. Resolution failures are ignored
// (the crawl proceeds with accountID 0), preserving the prior behaviour.
func resolveCrawlAccountID(db *store.Store, args map[string]any, orgID int64) int64 {
	accountID := argInt64(args, "account_id")
	if accountID <= 0 && orgID > 0 && db != nil {
		if pickedAccountID, err := pickReadyFacebookAccountIDForCrawl(db, orgID, argInt64(args, "user_id"), argString(args, "user_role")); err == nil && pickedAccountID > 0 {
			accountID = pickedAccountID
			args["account_id"] = pickedAccountID
		}
	}
	return accountID
}

// buildCrawlExtras collects the optional connector hints (market-signal gate,
// user prompt, and the P1.3E direct-post target) forwarded via Task.Extras.
func buildCrawlExtras(args map[string]any) map[string]any {
	extras := map[string]any{}
	if gate, ok := args["market_signal_gate"]; ok && gate != nil {
		extras["market_signal_gate"] = gate
	}
	if up := strings.TrimSpace(argString(args, "user_prompt")); up != "" {
		extras["user_prompt"] = up
	}
	// P1.3E direct-post target: when the caller pins an explicit permalink target (direct-post
	// intake), forward it to the connector so the extension extracts ONLY that post and fails
	// typed when it is not rendered. Presence of post_fbid OR canonical marks the targeted mode;
	// broad crawl/search never sets these args, so the extension keeps its feed behaviour.
	if dpFB := strings.TrimSpace(argString(args, "direct_post_post_fbid")); dpFB != "" || strings.TrimSpace(argString(args, "direct_post_canonical")) != "" {
		extras["direct_post_target"] = map[string]any{
			"post_fbid":     dpFB,
			"group_ref":     strings.TrimSpace(argString(args, "direct_post_group_ref")),
			"canonical_url": strings.TrimSpace(argString(args, "direct_post_canonical")),
		}
	}
	return extras
}

func openCrawlTaskID(intent string, sources []jobs.Source, args map[string]any) string {
	if taskID := argString(args, "_task_id"); strings.HasPrefix(taskID, "autocrawl-") {
		return taskID
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s|day=%s|", intent, time.Now().UTC().Format("2006-01-02"))
	for _, src := range sources {
		fmt.Fprintf(h, "%s:%s|", src.Type, src.URL)
	}
	fmt.Fprintf(h, "org=%d|account=%d", argInt64(args, "org_id"), argInt64(args, "account_id"))
	return fmt.Sprintf("open-crawl-%x", h.Sum(nil))[:27]
}

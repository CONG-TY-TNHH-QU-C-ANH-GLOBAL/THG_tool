package main

import (
	"strings"

	"github.com/thg/scraper/internal/crawler"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
)

// resolveCrawlRequest maps the raw args bag to a typed crawler.CrawlRequest, performing
// the exact same reads/resolutions submitOpenCrawl did inline. It stays in cmd because
// it depends on the package-main arg/prompt helpers + the RBAC account auto-pick; the
// execution core that consumes the request lives in internal/crawler (ARCHCM4b).
// Account resolution runs BEFORE the task id is computed because resolveCrawlAccountID
// writes the resolved account back into args and openCrawlTaskID hashes
// args["account_id"] — preserving the original evaluation order.
func resolveCrawlRequest(db *store.Store, intent string, sources []jobs.Source, args map[string]any) crawler.CrawlRequest {
	orgID := argInt64(args, "org_id")
	accountID := resolveCrawlAccountID(db, args, orgID) // writes args["account_id"]; must precede openCrawlTaskID
	return crawler.CrawlRequest{
		Intent:           intent,
		Sources:          sources,
		OrgID:            orgID,
		AccountID:        accountID,
		MaxItems:         resolveCrawlMaxItems(args),
		Keywords:         resolveCrawlKeywords(args),
		Extras:           buildCrawlExtras(args),
		TaskID:           openCrawlTaskID(intent, sources, args),
		IntentID:         argInt64(args, "_intent_id"),
		CursorLastPostID: strings.TrimSpace(argString(args, "_cursor_last_post_id")),
		CursorLastPostAt: parseRFC3339OrZero(argString(args, "_cursor_last_post_at")),
		SinceRunAt:       parseRFC3339OrZero(argString(args, "_since_run_at")),
		RecurringRun:     argBool(args, "_recurring_run"),
		IntervalMinutes:  int(argInt64(args, "interval_minutes")),
		Prompt:           argString(args, "user_prompt"),
		Name:             textutil.FirstNonEmpty(argString(args, "name"), argString(args, "query")),
	}
}

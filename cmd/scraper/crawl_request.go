package main

import (
	"strings"
	"time"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
)

// crawlRequest is the typed, fully-resolved input to the open-crawl execution core.
// It is produced from the raw args map by resolveCrawlRequest (the cmd-side arg/prompt
// resolution facade) so the execution path no longer threads map[string]any. The core
// that consumes it (submitCrawlRequest) is what moves to internal/crawler in ARCHCM4b;
// the resolution here stays in cmd because it depends on package-main arg/prompt
// helpers. This is a pure plumbing seam — it changes how inputs are passed, not what
// the runtime does (ARCHCM4a; behavior-preserving).
type crawlRequest struct {
	Intent           string
	Sources          []jobs.Source
	OrgID            int64
	AccountID        int64
	MaxItems         int
	Keywords         []string
	Extras           map[string]any
	TaskID           string
	IntentID         int64
	CursorLastPostID string
	CursorLastPostAt time.Time
	SinceRunAt       time.Time
	// Recurring-intent memory inputs (consumed only when a recurring intent is remembered).
	RecurringRun    bool
	IntervalMinutes int
	Prompt          string
	Name            string
}

// resolveCrawlRequest maps the raw args bag to a typed crawlRequest, performing the
// exact same reads/resolutions submitOpenCrawl did inline. Account resolution runs
// BEFORE the task id is computed because resolveCrawlAccountID writes the resolved
// account back into args and openCrawlTaskID hashes args["account_id"] — preserving
// the original evaluation order.
func resolveCrawlRequest(db *store.Store, intent string, sources []jobs.Source, args map[string]any) crawlRequest {
	orgID := argInt64(args, "org_id")
	accountID := resolveCrawlAccountID(db, args, orgID) // writes args["account_id"]; must precede openCrawlTaskID
	return crawlRequest{
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

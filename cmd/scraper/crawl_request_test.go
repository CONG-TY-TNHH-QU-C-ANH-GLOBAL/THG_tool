package main

import (
	"testing"
	"time"

	"github.com/thg/scraper/internal/jobs"
)

// crawlReqTestSources is a single recurring-eligible source used by the resolution tests.
var crawlReqTestSources = []jobs.Source{{Type: "facebook_group", URL: "https://facebook.com/groups/x", Label: "L"}}

// resolveCrawlRequest is the ARCHCM4a arg→typed seam. These characterization tests pin
// the exact mapping submitOpenCrawl did inline, so the de-arg (and the later
// internal/crawler move) stays behavior-preserving. An explicit account_id (>0) keeps
// resolveCrawlAccountID off the DB, so db can be nil — and it also pins the founder's
// "explicit account_id pass-through" invariant (#7): the chosen account is used as-is.
func TestResolveCrawlRequest_TypedMapping(t *testing.T) {
	since := "2026-06-01T00:00:00Z"
	args := map[string]any{
		"org_id":               int64(3),
		"account_id":           int64(55), // explicit → used as-is, no DB owner re-pick
		"_intent_id":           int64(9),
		"_task_id":             "autocrawl-9-100",
		"_cursor_last_post_id": "  p123  ",
		"_since_run_at":        since,
		"_recurring_run":       true,
		"interval_minutes":     30,
		"user_prompt":          "find cat tees",
		"name":                 "Mission A",
		"keywords":             "cat, tee",
	}
	req := resolveCrawlRequest(nil, "facebook_crawl", crawlReqTestSources, args)

	wantSince, _ := time.Parse(time.RFC3339, since)
	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"Intent", req.Intent, "facebook_crawl"},
		{"OrgID", req.OrgID, int64(3)},
		{"AccountID", req.AccountID, int64(55)},
		{"IntentID", req.IntentID, int64(9)},
		{"TaskID", req.TaskID, "autocrawl-9-100"},
		{"CursorLastPostID", req.CursorLastPostID, "p123"}, // trimmed
		{"SinceRunAt", req.SinceRunAt.Equal(wantSince), true},
		{"RecurringRun", req.RecurringRun, true},
		{"IntervalMinutes", req.IntervalMinutes, 30},
		{"Prompt", req.Prompt, "find cat tees"},
		{"Name", req.Name, "Mission A"},
		{"KeywordCount", len(req.Keywords), 2},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("resolveCrawlRequest %s = %v, want %v", c.field, c.got, c.want)
		}
	}
}

// MaxItems fallback chain: explicit max_items → limit → (prompt) → default 50.
func TestResolveCrawlRequest_MaxItemsFallback(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want int
	}{
		{"explicit max_items", map[string]any{"account_id": int64(1), "max_items": 5}, 5},
		{"limit fallback", map[string]any{"account_id": int64(1), "limit": 7}, 7},
		{"default 50", map[string]any{"account_id": int64(1)}, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := resolveCrawlRequest(nil, "facebook_crawl", crawlReqTestSources, tc.args)
			if req.MaxItems != tc.want {
				t.Errorf("MaxItems = %d, want %d", req.MaxItems, tc.want)
			}
		})
	}
}

// Name resolves FirstNonEmpty(name, query) — same as the old inline resolution.
func TestResolveCrawlRequest_NameFirstNonEmpty(t *testing.T) {
	cases := []struct {
		name     string
		nameArg  string
		queryArg string
		wantName string
	}{
		{"name wins", "N", "Q", "N"},
		{"query fallback", "", "Q", "Q"},
		{"both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"account_id": int64(1), "name": tc.nameArg, "query": tc.queryArg}
			req := resolveCrawlRequest(nil, "facebook_crawl", crawlReqTestSources, args)
			if req.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", req.Name, tc.wantName)
			}
		})
	}
}

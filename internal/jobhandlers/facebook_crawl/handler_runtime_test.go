package facebookcrawl

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/scoring"
)

// PR31C crawl-runtime safety net: characterize the Facebook crawl Handler's
// browser-error → status/reason mapping without a real browser or DB. The
// Runtime interface is a single FetchBatch method, so a tiny in-test fake drives
// every anti-bot / drift / failure branch; appStore + jobStore are nil-safe in
// Handle, so the error paths never touch the store. Behavior-preserving recon.

// fakeCrawlRuntime is a one-method runtime.Runtime stand-in: it returns the same
// (items, err) on every FetchBatch call. err alone exercises the handler's
// error-mapping branches before any ingest/store work runs.
type fakeCrawlRuntime struct {
	items []runtime.RawItem
	err   error
}

func (f fakeCrawlRuntime) FetchBatch(context.Context, string, int, int) ([]runtime.RawItem, error) {
	return f.items, f.err
}

// runHandle builds a store-free handler (nil jobStore + appStore are guarded in
// Handle) around rt and runs one crawl job over a single group source.
func runHandle(t *testing.T, rt runtime.Runtime) (string, error) {
	t.Helper()
	h := New(rt, scoring.New(scoring.DefaultConfig()), nil, nil)
	payload, err := json.Marshal(jobs.Task{
		TaskID:    "t1",
		OrgID:     1,
		Intent:    "scrape_group",
		CrawlPlan: jobs.CrawlPlan{Sources: []jobs.Source{{Type: "group", URL: "https://facebook.com/groups/123"}}, MaxItems: 50},
	})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	return h.Handle(context.Background(), &jobs.Job{ID: 1, TaskID: "t1", Intent: "scrape_group", Payload: string(payload)})
}

// TestHandle_RuntimeErrorMapping pins the anti-bot / drift contract: a checkpoint
// is a HUMAN_REQUIRED gate (never a silent retry); logout / banned abort without
// queueing human work; context-drift aborts rather than scraping off-target. Each
// returns a terminal JSON status and a nil error (handled, not propagated).
func TestHandle_RuntimeErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		code runtime.CDPErrorCode
		want string
	}{
		{"checkpoint is human_required", runtime.ErrFacebookCheckpoint, `{"status":"human_required","reason":"facebook_checkpoint"}`},
		{"logout aborts", runtime.ErrFacebookLogout, `{"status":"aborted","reason":"facebook_logout"}`},
		{"banned aborts", runtime.ErrFacebookBanned, `{"status":"aborted","reason":"facebook_banned"}`},
		{"context drift aborts", runtime.ErrFacebookContextDrift, `{"status":"aborted","reason":"context_drift"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runHandle(t, fakeCrawlRuntime{err: runtime.CDPError{Code: tc.code}})
			if err != nil {
				t.Fatalf("anti-bot signal must be handled (nil err); got %v", err)
			}
			if got != tc.want {
				t.Fatalf("status mapping:\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestHandle_GenericFetchErrorPropagates pins the fallback: a non-anti-bot,
// non-drift fetch failure (transient CDP error or a plain error) is NOT a terminal
// status — it propagates as an error so the job/retry layer decides, and no
// partial result is emitted.
func TestHandle_GenericFetchErrorPropagates(t *testing.T) {
	for _, fetchErr := range []error{
		runtime.CDPError{Code: runtime.ErrNavigationTimeout},
		errors.New("boom"),
	} {
		t.Run(fetchErr.Error(), func(t *testing.T) {
			got, err := runHandle(t, fakeCrawlRuntime{err: fetchErr})
			if err == nil {
				t.Fatalf("generic fetch error must propagate; got result=%q nil err", got)
			}
			if got != "" {
				t.Fatalf("error path must emit no result; got %q", got)
			}
		})
	}
}

// TestHandle_NoRuntimeConfigured pins the fail-loud guard: with no browser
// session allocator and no fallback runtime, Handle refuses to run rather than
// silently producing zero leads.
func TestHandle_NoRuntimeConfigured(t *testing.T) {
	got, err := runHandle(t, nil)
	if err == nil {
		t.Fatalf("nil runtime must error, not run; got %q", got)
	}
}

// TestHandle_EmptyResultCompletes pins the normal exhausted-source path: an empty
// fetch is a clean completion (valid result dataset, nil error), NOT an error and
// NOT a human_required gate.
func TestHandle_EmptyResultCompletes(t *testing.T) {
	got, err := runHandle(t, fakeCrawlRuntime{}) // nil items, nil err → exhausted
	if err != nil {
		t.Fatalf("empty source must complete cleanly; got %v", err)
	}
	if strings.Contains(got, "human_required") || strings.Contains(got, "aborted") {
		t.Fatalf("empty completion must not be a gate/abort; got %s", got)
	}
	var ds map[string]any
	if err := json.Unmarshal([]byte(got), &ds); err != nil {
		t.Fatalf("result must be a valid dataset JSON: %v (got %q)", err, got)
	}
}

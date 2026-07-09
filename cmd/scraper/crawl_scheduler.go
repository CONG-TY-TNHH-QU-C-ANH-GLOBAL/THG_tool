package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/session/accountsafety"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
)

func runCrawlIntentScheduler(ctx context.Context, db *store.Store, jobStore *jobs.Store, coord *accountsafety.Coordinator, tickEvery time.Duration) {
	if db == nil || jobStore == nil || coord == nil {
		return
	}
	if tickEvery <= 0 {
		tickEvery = time.Minute
	}
	run := func() {
		if err := scheduleDueCrawlIntents(ctx, db, jobStore, coord); err != nil {
			log.Printf("[CrawlIntent] scheduler error: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func scheduleDueCrawlIntents(ctx context.Context, db *store.Store, jobStore *jobs.Store, coord *accountsafety.Coordinator) error {
	now := time.Now().UTC()
	// Account Safety Coordinator (PR-C4): claim only as many due intents as the
	// machine can safely run now (default budget = 1). Un-claimed due intents keep
	// their next_run_at and are re-picked on a later tick — a clean cross-tick
	// queue that needs NO change to ClaimDueIntents semantics. This never raises
	// concurrency; it only lowers it.
	free := coord.FreeSlots(now)
	if free <= 0 {
		return nil
	}
	intents, err := db.Crawl().ClaimDueIntents(ctx, now, free)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		taskID := recurringCrawlTaskID(intent.ID, now, intent.IntervalMinutes)
		// Reliability Track invariant: NEVER silently fall back to a "first ready"
		// account. A mission without an explicit account_id is a misconfiguration
		// (e.g. legacy intents 7/9 created before PR-A required account selection)
		// — skip it with a typed reason so the operator fixes the mission, instead
		// of piling every account-less mission onto one connector.
		accountID := intent.AccountID
		if accountID <= 0 {
			// account_not_selected is a PERMANENT misconfiguration (not a
			// transient run failure) — stop the intent IMMEDIATELY rather than
			// re-firing every interval. Record the reason, then fail the intent
			// so ClaimDueIntents (WHERE status='active') never picks it again.
			// The user fixes it by deleting + recreating with an account (PR-A
			// makes the form require one).
			errMsg := "account_not_selected: mission has no account_id — delete and recreate the mission, choosing the account that should run it"
			_ = db.Crawl().MarkIntentRunResult(ctx, intent.ID, taskID, errMsg)
			_ = db.Crawl().SetIntentStatus(ctx, intent.OrgID, intent.ID, "failed")
			log.Printf("[CrawlIntent] failed intent=%d org=%d: account_not_selected (stopped)", intent.ID, intent.OrgID)
			continue
		}
		args := map[string]any{
			"org_id":         intent.OrgID,
			"account_id":     accountID,
			"keywords":       strings.Join(intent.Keywords, ", "),
			"max_items":      intent.MaxItems,
			"user_prompt":    intent.Prompt,
			"_recurring_run": true,
			"_task_id":       taskID,
			// Soft cursor: crawler may skip content older than the previous
			// run / the explicit cursor when honoring this. See
			// project_scheduled_intelligence.md gap #2.
			"_intent_id":           intent.ID,
			"_since_run_at":        formatRFC3339OrEmpty(intent.LastRunAt),
			"_cursor_last_post_id": intent.CursorLastPostID,
			"_cursor_last_post_at": formatRFC3339OrEmpty(intent.CursorLastPostAt),
		}
		source := jobs.Source{Type: intent.SourceType, URL: intent.SourceURL, Label: textutil.FirstNonEmpty(intent.SourceLabel, "recurring_intent")}
		result, submitErr := submitOpenCrawl(ctx, db, jobStore, intent.Intent, []jobs.Source{source}, args)
		errMsg := ""
		if submitErr != nil {
			errMsg = submitErr.Error()
		}
		if err := db.Crawl().MarkIntentRunResult(ctx, intent.ID, taskID, errMsg); err != nil {
			log.Printf("[CrawlIntent] mark result failed intent=%d: %v", intent.ID, err)
		}
		if submitErr != nil {
			log.Printf("[CrawlIntent] run failed intent=%d task=%s: %v", intent.ID, taskID, submitErr)
			continue
		}
		// Consume a machine slot until the crawl result frees it (PR-C4B
		// result-feedback) or the coordinator's stale-running fallback does.
		coord.MarkRunning(accountID, now)
		log.Printf("[CrawlIntent] scheduled intent=%d task=%s: %s (safety: active=%d)", intent.ID, taskID, result, coord.Snapshot(now).Active)
	}
	return nil
}

func formatRFC3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parseRFC3339OrZero(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func recurringCrawlTaskID(intentID int64, now time.Time, intervalMinutes int) string {
	if intervalMinutes <= 0 {
		intervalMinutes = 30
	}
	bucketSeconds := int64(intervalMinutes * 60)
	if bucketSeconds <= 0 {
		bucketSeconds = 1800
	}
	return fmt.Sprintf("autocrawl-%d-%d", intentID, now.UTC().Unix()/bucketSeconds)
}

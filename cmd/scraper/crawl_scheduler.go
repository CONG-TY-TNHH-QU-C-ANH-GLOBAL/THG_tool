package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store/crawl"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
)

func rememberRecurringCrawlIntents(ctx context.Context, db *store.Store, task *jobs.Task, args map[string]any) {
	if db == nil || task == nil || task.OrgID <= 0 || task.AccountID <= 0 {
		return
	}
	prompt := argString(args, "user_prompt")
	intervalMinutes := int(argInt64(args, "interval_minutes"))
	maxItems := task.CrawlPlan.MaxItems
	for _, src := range task.CrawlPlan.Sources {
		if !isRecurringCrawlSource(src) {
			continue
		}
		intent, err := db.Crawl().UpsertIntent(ctx, crawl.Intent{
			OrgID:           task.OrgID,
			AccountID:       task.AccountID,
			Name:            textutil.FirstNonEmpty(argString(args, "name"), argString(args, "query")),
			Prompt:          prompt,
			Intent:          task.Intent,
			SourceType:      src.Type,
			SourceURL:       src.URL,
			SourceLabel:     src.Label,
			Keywords:        task.Keywords,
			IntervalMinutes: intervalMinutes,
			MaxItems:        maxItems,
		})
		if err != nil {
			log.Printf("[CrawlIntent] remember failed org=%d account=%d source=%s: %v", task.OrgID, task.AccountID, src.URL, err)
			continue
		}
		log.Printf("[CrawlIntent] remembered org=%d account=%d intent=%d interval=%dm source=%s", intent.OrgID, intent.AccountID, intent.ID, intent.IntervalMinutes, intent.SourceURL)
	}
}

func isRecurringCrawlSource(src jobs.Source) bool {
	switch strings.ToLower(strings.TrimSpace(src.Type)) {
	case "facebook_group", "facebook_search", "web_url":
		return strings.TrimSpace(src.URL) != ""
	default:
		return false
	}
}

func runCrawlIntentScheduler(ctx context.Context, db *store.Store, jobStore *jobs.Store, tickEvery time.Duration) {
	if db == nil || jobStore == nil {
		return
	}
	if tickEvery <= 0 {
		tickEvery = time.Minute
	}
	run := func() {
		if err := scheduleDueCrawlIntents(ctx, db, jobStore); err != nil {
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

func scheduleDueCrawlIntents(ctx context.Context, db *store.Store, jobStore *jobs.Store) error {
	now := time.Now().UTC()
	intents, err := db.Crawl().ClaimDueIntents(ctx, now, 10)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		accountID := intent.AccountID
		if accountID <= 0 {
			if picked, pickErr := pickReadyFacebookAccountIDForCrawl(db, intent.OrgID); pickErr == nil {
				accountID = picked
			}
		}
		taskID := recurringCrawlTaskID(intent.ID, now, intent.IntervalMinutes)
		if accountID <= 0 {
			errMsg := "no ready Facebook account for recurring crawl"
			_ = db.Crawl().MarkIntentRunResult(ctx, intent.ID, taskID, errMsg)
			log.Printf("[CrawlIntent] skipped intent=%d org=%d: %s", intent.ID, intent.OrgID, errMsg)
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
			"_intent_id":              intent.ID,
			"_since_run_at":           formatRFC3339OrEmpty(intent.LastRunAt),
			"_cursor_last_post_id":    intent.CursorLastPostID,
			"_cursor_last_post_at":    formatRFC3339OrEmpty(intent.CursorLastPostAt),
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
		log.Printf("[CrawlIntent] scheduled intent=%d task=%s: %s", intent.ID, taskID, result)
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

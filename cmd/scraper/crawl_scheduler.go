package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
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
		intent, err := db.UpsertCrawlIntent(ctx, store.CrawlIntent{
			OrgID:           task.OrgID,
			AccountID:       task.AccountID,
			Name:            firstNonEmpty(argString(args, "name"), argString(args, "query")),
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
	intents, err := db.ClaimDueCrawlIntents(ctx, now, 10)
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
			_ = db.MarkCrawlIntentRunResult(ctx, intent.ID, taskID, errMsg)
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
		}
		source := jobs.Source{Type: intent.SourceType, URL: intent.SourceURL, Label: firstNonEmpty(intent.SourceLabel, "recurring_intent")}
		result, submitErr := submitOpenCrawl(ctx, db, jobStore, intent.Intent, []jobs.Source{source}, args)
		errMsg := ""
		if submitErr != nil {
			errMsg = submitErr.Error()
		}
		if err := db.MarkCrawlIntentRunResult(ctx, intent.ID, taskID, errMsg); err != nil {
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

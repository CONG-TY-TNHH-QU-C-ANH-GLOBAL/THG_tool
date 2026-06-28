package crawler

import (
	"context"
	"log"
	"strings"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/crawl"
)

// rememberRecurringCrawlIntents persists a recurring crawl intent for each recurring-
// eligible source, from already-resolved typed inputs. Behavior (and UpsertIntent
// fields) are unchanged from the cmd/scraper version.
func rememberRecurringCrawlIntents(ctx context.Context, db *store.Store, task *jobs.Task, prompt, name string, intervalMinutes int) {
	if db == nil || task == nil || task.OrgID <= 0 || task.AccountID <= 0 {
		return
	}
	maxItems := task.CrawlPlan.MaxItems
	for _, src := range task.CrawlPlan.Sources {
		if !isRecurringCrawlSource(src) {
			continue
		}
		intent, err := db.Crawl().UpsertIntent(ctx, crawl.Intent{
			OrgID:           task.OrgID,
			AccountID:       task.AccountID,
			Name:            name,
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

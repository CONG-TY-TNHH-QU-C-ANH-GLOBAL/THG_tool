package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
)

// CrawlRequest is the typed, fully-resolved input to the open-crawl execution core.
// cmd/scraper builds it (resolveCrawlRequest, the raw args/prompt facade) and passes
// it here. Moving it out of the composition root is ARCHCM4b; the resolution + RBAC
// account auto-pick stay in cmd.
type CrawlRequest struct {
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

// SubmitCrawlRequest is the typed open-crawl execution core: build the Task, remember a
// recurring intent when configured, then run the connector-dispatch ladder with the
// server-job fallback. Behavior is byte-for-byte what cmd/scraper's submitCrawlRequest
// did before the ARCHCM4b move.
func SubmitCrawlRequest(ctx context.Context, db *store.Store, jobStore *jobs.Store, req CrawlRequest) (string, error) {
	task := &jobs.Task{
		SchemaVersion: "1",
		TaskID:        req.TaskID,
		OrgID:         req.OrgID,
		AccountID:     req.AccountID,
		IntentID:      req.IntentID,
		Intent:        req.Intent,
		Keywords:      req.Keywords,
		CrawlPlan: jobs.CrawlPlan{
			Sources:          req.Sources,
			MaxItems:         req.MaxItems,
			BatchSize:        20,
			CursorLastPostID: req.CursorLastPostID,
			CursorLastPostAt: req.CursorLastPostAt,
			SinceRunAt:       req.SinceRunAt,
		},
		Filters: jobs.Filters{Keywords: req.Keywords, MinContentLength: 20, KeywordMinScore: 0},
		ScoringConfig: jobs.ScoringConfig{
			HotThreshold:  70,
			WarmThreshold: 40,
			Weights: jobs.ScoringWeights{
				KeywordRelevance: 0.4,
				Engagement:       0.2,
				ContentQuality:   0.4,
			},
		},
		RetryPolicy:         jobs.RetryPolicy{MaxAttempts: 3, BackoffMs: 1000},
		ExecutionMode:       "async",
		OutputSchema:        "open_crawler_v1",
		OutputSchemaVersion: "1",
		Extras:              req.Extras,
	}
	if db != nil && !req.RecurringRun && req.IntervalMinutes > 0 {
		rememberRecurringCrawlIntents(ctx, db, task, req.Prompt, req.Name, req.IntervalMinutes)
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	if db != nil {
		if result, routed, err := submitConnectorCrawl(ctx, db, task, string(payload)); routed {
			return result, err
		}
	}
	job, err := jobStore.Submit(ctx, task, string(payload))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("da tao crawler job #%d task=%s intent=%s", job.ID, job.TaskID, req.Intent), nil
}

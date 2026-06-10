package main

import (
	"context"
	"log"
	"time"

	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/store"
)

// Async comment reverify scheduler (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-A). Periodic
// maintenance loop (mirrors runAutoArchiveScheduler): find submitted-unverified comments
// older than the reverify delay and enqueue them onto the reverify queue. The extension
// polls the queue (GET /api/agent/reverify/claim), re-opens the post, searches for the
// comment, and reports back — at which point the backend appends the append-only
// correction. The scheduler itself only SCHEDULES; it never touches the ledger.

func runCommentReverifyScheduler(ctx context.Context, db *store.Store, cfg *config.Config) {
	if db == nil || cfg == nil {
		return
	}
	tickEvery := time.Duration(cfg.CommentReverifyIntervalMin) * time.Minute
	if tickEvery <= 0 {
		tickEvery = 2 * time.Minute
	}
	delay := time.Duration(cfg.CommentReverifyDelayMin) * time.Minute
	if delay <= 0 {
		delay = 3 * time.Minute
	}
	run := func() {
		if err := scheduleDueReverifies(ctx, db, delay); err != nil {
			log.Printf("[Reverify] scheduler error: %v", err)
		}
	}
	run() // once at startup
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

// scheduleDueReverifies enqueues every eligible submitted-unverified comment that is past
// the reverify delay and not already scheduled. ScheduleReverify is idempotent.
func scheduleDueReverifies(ctx context.Context, db *store.Store, delay time.Duration) error {
	now := time.Now()
	jobs, err := db.Coordination().FindReverifyEligible(ctx, now.Add(-delay), 100)
	if err != nil {
		return err
	}
	scheduled := 0
	for _, j := range jobs {
		// Claimable immediately: the delay was already enforced by the eligibility filter.
		if err := db.Coordination().ScheduleReverify(ctx, j, now); err != nil {
			log.Printf("[Reverify] schedule outbound=%d: %v", j.OutboundID, err)
			continue
		}
		scheduled++
	}
	if scheduled > 0 {
		log.Printf("[Reverify] scheduled=%d reverify jobs", scheduled)
	}
	return nil
}

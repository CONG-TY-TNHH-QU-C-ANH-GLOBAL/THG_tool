package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

// Direct-post intake process manager (P1 PR-2). A durable, crash-safe DB poller
// (mirrors runCommentReverifyScheduler): it claims due direct_post_comment_workflows
// (lease), observes whether the imported POST lead now exists (GetPostLeadByRef), and
// on the FIRST observation queues the comment through the existing queueLeadOutreach
// gates. NO in-memory callback as source of truth; NO Telegram send here (the lead's
// own creation notification fires once); NO comment before the lead exists.

const (
	directPostIntakeInterval  = 30 * time.Second
	directPostIntakeBatchSize = 20
)

func directPostWorkerID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("dpi-%s-%d", host, os.Getpid())
}

// runDirectPostIntakeScheduler polls until ctx is cancelled. It exits cleanly on
// shutdown (stops the ticker, stops claiming/querying); any workflow already leased
// is recovered via lease expiry on the next process — no risky in-memory cleanup.
func runDirectPostIntakeScheduler(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, notify func(string)) {
	if db == nil || msgGen == nil {
		return // queueLeadOutreach needs a generator; nothing to drive without it
	}
	workerID := directPostWorkerID()
	run := func() { processDueDirectPostWorkflows(ctx, db, msgGen, notify, workerID) }
	run() // once at startup
	ticker := time.NewTicker(directPostIntakeInterval)
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

func processDueDirectPostWorkflows(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, notify func(string), workerID string) {
	if ctx.Err() != nil {
		return
	}
	now := time.Now().UTC()
	claimed, err := db.Coordination().ClaimDueDirectPostCommentWorkflows(
		ctx, now, workerID, now.Add(coordination.DPDefaultLeaseDuration), directPostIntakeBatchSize)
	if err != nil {
		log.Printf("[DirectPostIntake] claim error: %v", err)
		return
	}
	for _, w := range claimed {
		if ctx.Err() != nil {
			return // graceful shutdown: stop mid-batch; leased rows recover via lease expiry
		}
		advanceDirectPostWorkflow(ctx, db, msgGen, notify, w)
	}
}

// advanceDirectPostWorkflow runs one CAS-guarded step for a claimed workflow.
func advanceDirectPostWorkflow(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, notify func(string), w *coordination.DirectPostCommentWorkflow) {
	lead, err := db.Leads().GetPostLeadByRef(ctx, w.OrgID, w.PostFBID, w.CanonicalPostURL)
	if err != nil {
		log.Printf("[DirectPostIntake] lookup org=%d wf=%d: %v", w.OrgID, w.ID, err)
		return
	}
	if lead == nil {
		// Import not visible yet. No job-status oracle → bounded retry with backoff,
		// then a typed actionable failure (no hallucinated comment, no silent drop).
		now := time.Now().UTC()
		if w.RetryCount >= coordination.DPMaxRetryCount {
			// Honest terminal reason: we only observe the lead, not the job — so we say
			// "lead not observed", NOT "import failed" (which we can't confirm).
			_, _ = db.Coordination().MarkDirectPostFailed(ctx, w.OrgID, w.ID,
				coordination.DPErrLeadNotObserved, "post lead not observed after max retries")
			return
		}
		delay := coordination.DPBaseRetryDelay << uint(w.RetryCount)
		_, _ = db.Coordination().ScheduleDirectPostRetry(ctx, w.OrgID, w.ID, now.Add(delay),
			coordination.DPStatusImporting, "awaiting single-post import")
		return
	}
	// Lead exists → CAS to lead_created (idempotent: a racing poller that already
	// advanced it makes this a clean no-op, so the comment is never queued twice).
	if ok, _ := db.Coordination().MarkDirectPostLeadCreated(ctx, w.OrgID, w.ID, lead.ID); !ok {
		return
	}
	qargs := map[string]any{
		"org_id":    w.OrgID,
		"user_id":   w.RequestedByUserID,
		"user_role": w.UserRole,
		"lead_id":   lead.ID,
		"max_items": int64(1),
	}
	if w.AccountID > 0 {
		qargs["account_id"] = w.AccountID
	}
	if _, err := queueLeadOutreach(ctx, db, msgGen, "comment", qargs, notify); err != nil {
		// Typed reason only — never the raw error (no secrets in error_message).
		_, _ = db.Coordination().MarkDirectPostFailed(ctx, w.OrgID, w.ID,
			coordination.DPStatusCommentFailed, "comment queue failed")
		return
	}
	_, _ = db.Coordination().MarkDirectPostCommentQueued(ctx, w.OrgID, w.ID)
	_, _ = db.Coordination().MarkDirectPostCompleted(ctx, w.OrgID, w.ID)
}

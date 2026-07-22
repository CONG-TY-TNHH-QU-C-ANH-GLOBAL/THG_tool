package main

import (
	"context"
	"log"
	"time"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/services/facebook/crawlcampaign"
	"github.com/thg/scraper/internal/session/accountsafety"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/crawlrun"
)

// PR-M4A lifecycle/wiring for the fresh-lead campaign orchestrator. Default-off
// and Postgres-only; the adapters it wires live in crawl_campaign_orchestrator.go.

// startFreshLeadCampaignOrchestrator launches the campaign orchestrator when
// enabled and the runtime supports it, else it is a no-op. Kept out of main() so
// the composition root stays flat. enabled is cfg.FreshLeadCampaignsEnabled
// (FRESH_LEAD_CAMPAIGNS_ENABLED, default false).
func startFreshLeadCampaignOrchestrator(ctx context.Context, enabled bool, db *store.Store, jobStore *jobs.Store, coord *accountsafety.Coordinator) {
	if !enabled {
		return
	}
	orch := newFreshLeadCampaignOrchestrator(db, jobStore, coord)
	if orch == nil {
		log.Println("⚠️  FRESH_LEAD_CAMPAIGNS_ENABLED set but runtime store is not Postgres — campaign orchestrator not started (fail-closed)")
		return
	}
	go runFreshLeadCampaignScheduler(ctx, orch, time.Minute)
	log.Println("✅ Fresh-lead campaign orchestrator started (FRESH_LEAD_CAMPAIGNS_ENABLED=1; shared machine budget=1)")
}

// newFreshLeadCampaignOrchestrator wires the fresh-lead campaign orchestrator, or
// returns nil when the runtime store is not Postgres (the campaign tables are
// Postgres-only, so the orchestrator fails closed rather than spin uselessly).
// The coordinator is the SAME instance the legacy crawl scheduler uses, so the
// machine crawl budget (default 1) is shared across both — never doubled.
func newFreshLeadCampaignOrchestrator(db *store.Store, jobStore *jobs.Store, coord *accountsafety.Coordinator) *crawlcampaign.Orchestrator {
	if db == nil || jobStore == nil || coord == nil || db.Dialect().Name() != "postgres" {
		return nil
	}
	runs := crawlrun.NewStore(db.DB(), db.Dialect())
	return crawlcampaign.New(crawlcampaign.Deps{
		Pools:      campaignRunStore{runs},
		Enqueuer:   campaignRunStore{runs},
		Claimer:    campaignRunStore{runs},
		Recoverer:  campaignRunStore{runs},
		Safety:     campaignSafetyGate{coord},
		Readiness:  campaignReadinessGate{db},
		Dispatcher: campaignDispatcher{db: db, jobStore: jobStore, runs: runs},
		Logf:       log.Printf,
	})
}

// runFreshLeadCampaignScheduler ticks the orchestrator on the server clock,
// mirroring runCrawlIntentScheduler. A tick error is logged, never fatal; the
// next tick retries.
func runFreshLeadCampaignScheduler(ctx context.Context, orch *crawlcampaign.Orchestrator, tickEvery time.Duration) {
	if orch == nil {
		return
	}
	if tickEvery <= 0 {
		tickEvery = time.Minute
	}
	run := func() {
		if err := orch.RunOnce(ctx, time.Now().UTC()); err != nil {
			log.Printf("[FreshLeadCampaign] orchestrator tick error: %v", err)
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

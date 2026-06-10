package main

import (
	"context"
	"log"
	"time"

	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/store"
)

// Auto-archive scheduler (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-3). A periodic
// maintenance loop, mirroring runCrawlIntentScheduler: tick, enumerate orgs with live
// leads, run the org-scoped ArchiveSweep. No hard delete — it only flips archived_at;
// the engagement ledger is preserved for dedup/coverage history.

// lifecyclePolicyFromConfig maps env-tunable config to the domain policy.
func lifecyclePolicyFromConfig(cfg *config.Config) models.LeadLifecyclePolicy {
	return models.LeadLifecyclePolicy{
		StaleAfterDays:        cfg.StaleAfterDays,
		ArchiveAfterDays:      cfg.ArchiveAfterDays,
		EvidenceRetentionDays: cfg.EvidenceRetentionDays,
		RawCrawlRetentionDays: cfg.RawCrawlRetentionDays,
		FollowupWindow:        models.DefaultFollowupWindow,
		VerificationCooldown:  time.Duration(cfg.VerificationCooldownMin) * time.Minute,
	}
}

func runAutoArchiveScheduler(ctx context.Context, db *store.Store, cfg *config.Config) {
	if db == nil || cfg == nil {
		return
	}
	tickEvery := time.Duration(cfg.ArchiveIntervalMin) * time.Minute
	if tickEvery <= 0 {
		tickEvery = 6 * time.Hour
	}
	policy := lifecyclePolicyFromConfig(cfg)
	run := func() {
		if err := sweepAllOrgsForArchive(ctx, db, policy); err != nil {
			log.Printf("[AutoArchive] scheduler error: %v", err)
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

// sweepAllOrgsForArchive runs the archive sweep for every org that still has live leads.
func sweepAllOrgsForArchive(ctx context.Context, db *store.Store, policy models.LeadLifecyclePolicy) error {
	orgIDs, err := db.Leads().OrgIDsWithActiveLeads(ctx)
	if err != nil {
		return err
	}
	coveragePolicy := models.DefaultCoveragePolicy()
	for _, orgID := range orgIDs {
		// Website is only used by coverage CONTENT detection, irrelevant to archiving —
		// pass "" so the sweep stays free of per-org knowledge lookups.
		start := time.Now()
		report, err := db.Leads().ArchiveSweep(ctx, orgID, policy, coveragePolicy, "")
		if err != nil {
			log.Printf("[AutoArchive] org=%d sweep error: %v", orgID, err)
			continue
		}
		// Structured metric on the typed taxonomy so the runtime dashboard can chart
		// archive volume + per-reason breakdown per tenant. Emitted whenever the sweep
		// looked at anything, even with 0 archived (proves it ran, tenant-scoped).
		if report.Scanned > 0 {
			events.Info(ctx, events.LeadArchiveSweep,
				events.FieldOrgID, orgID,
				events.FieldScanned, report.Scanned,
				events.FieldArchived, report.Archived,
				events.FieldReasons, report.ByReason,
				events.FieldDurationMS, time.Since(start).Milliseconds(),
			)
		}
	}
	return nil
}

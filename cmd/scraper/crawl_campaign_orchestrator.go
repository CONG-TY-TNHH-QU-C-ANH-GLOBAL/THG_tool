package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/crawler"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/readiness"
	"github.com/thg/scraper/internal/services/facebook/crawlcampaign"
	"github.com/thg/scraper/internal/session/accountsafety"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/crawlrun"
	"github.com/thg/scraper/internal/textutil"
)

// PR-M4A composition root: the thin adapters that map the merged durable store,
// the shared Account Safety Coordinator, the readiness primitive, and the
// existing Facebook crawl command path onto the pure crawlcampaign orchestrator
// ports. Nothing here owns policy; the orchestrator does. Lifecycle/wiring lives
// in crawl_campaign_wiring.go.

// campaignRunStore adapts crawlrun.Store to the pool/enqueue/claim/recover ports,
// translating persistence types to the orchestrator's domain DTOs.
type campaignRunStore struct{ runs *crawlrun.Store }

func (a campaignRunStore) ActiveCampaignPools(ctx context.Context) ([]crawlcampaign.OrgPool, error) {
	pools, err := a.runs.ActiveCampaignPools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]crawlcampaign.OrgPool, len(pools))
	for i, p := range pools {
		out[i] = crawlcampaign.OrgPool{OrgID: p.OrgID, AccountIDs: p.AccountIDs}
	}
	return out, nil
}

func (a campaignRunStore) EnqueueDueRuns(ctx context.Context, orgID int64, now time.Time) error {
	_, err := a.runs.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: orgID, Now: now})
	return err
}

func (a campaignRunStore) ClaimNextRun(ctx context.Context, orgID, accountID int64, now time.Time) (crawlcampaign.PooledClaim, bool, error) {
	claimed, ok, err := a.runs.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: orgID, AccountID: accountID, Now: now})
	if err != nil || !ok {
		return crawlcampaign.PooledClaim{}, ok, err
	}
	return crawlcampaign.PooledClaim{
		Fence:         crawlcampaign.RunFence{OrgID: orgID, RunID: claimed.RunID, Attempt: claimed.Attempt},
		CampaignID:    claimed.CampaignID,
		SourceID:      claimed.SourceID,
		AccountID:     claimed.AccountID,
		FreshCutoffAt: claimed.FreshCutoffAt,
	}, true, nil
}

func (a campaignRunStore) RecoverDispatchFailure(ctx context.Context, fence crawlcampaign.RunFence, accountID int64, now time.Time) error {
	_, err := a.runs.RecoverDispatchFailure(ctx, crawlrun.RecoverDispatchFailureInput{
		Fence:             crawlrun.Fence{OrgID: fence.OrgID, RunID: fence.RunID, Attempt: fence.Attempt},
		ExpectedAccountID: accountID,
		Now:               now,
	})
	return err
}

// campaignSafetyGate adapts the shared Account Safety Coordinator: reserve is a
// running mark, release finishes the run with the given reason.
type campaignSafetyGate struct{ coord *accountsafety.Coordinator }

func (s campaignSafetyGate) Eligible(accountID int64, now time.Time) bool {
	return s.coord.IsAccountEligible(accountID, now)
}
func (s campaignSafetyGate) FreeSlots(now time.Time) int { return s.coord.FreeSlots(now) }
func (s campaignSafetyGate) Reserve(accountID int64, now time.Time) {
	s.coord.MarkRunning(accountID, now)
}
func (s campaignSafetyGate) Release(accountID int64, reason string, now time.Time) {
	s.coord.Finish(accountID, reason, now)
}

// campaignReadinessGate answers pre-claim readiness through the shared readiness
// primitive as the org-wide scheduler (userID 0 skips the per-user ownership
// gate, matching the recurring-intent scheduler's org-wide auto-pick).
type campaignReadinessGate struct{ db *store.Store }

func (r campaignReadinessGate) Ready(ctx context.Context, orgID, accountID int64) bool {
	reason, _ := readiness.EvaluateCrawlAccountReadiness(ctx, r.db, orgID, 0, "", accountID)
	return reason == readiness.ReadinessReady
}

// campaignDispatcher hydrates the claimed run's source and sends it through the
// existing crawl command path, threading the fence into the task so a later
// result-ingest (PR-M5) can correlate the reported result back to the run.
type campaignDispatcher struct {
	db       *store.Store
	jobStore *jobs.Store
	runs     *crawlrun.Store
}

func (d campaignDispatcher) Dispatch(ctx context.Context, claim crawlcampaign.PooledClaim) error {
	info, err := d.runs.DispatchInfo(ctx, claim.Fence.OrgID, claim.CampaignID, claim.SourceID)
	if err != nil {
		return fmt.Errorf("dispatch info run=%d: %w", claim.Fence.RunID, err)
	}
	if strings.TrimSpace(info.SourceURL) == "" {
		return fmt.Errorf("run=%d source=%d has no crawl URL", claim.Fence.RunID, claim.SourceID)
	}
	req := crawler.CrawlRequest{
		Intent:    "facebook_crawl",
		Sources:   []jobs.Source{{Type: "facebook_group", URL: info.SourceURL, Label: textutil.FirstNonEmpty(info.SourceLabel, "fresh_lead_campaign")}},
		OrgID:     claim.Fence.OrgID,
		AccountID: claim.AccountID,
		MaxItems:  info.MaxItems,
		TaskID:    fmt.Sprintf("fresh-lead-%d-%d-%d", claim.Fence.OrgID, claim.Fence.RunID, claim.Fence.Attempt),
		Extras:    freshLeadDispatchExtras(claim),
	}
	_, err = crawler.SubmitCrawlRequest(ctx, d.db, d.jobStore, req)
	return err
}

// freshLeadDispatchExtras carries the fence + fresh-cutoff on the task so both
// the connector envelope (which embeds the full task) and the server-job payload
// preserve the run correlation for PR-M5 ingest. Underscore-prefixed keys match
// the recurring-intent convention.
func freshLeadDispatchExtras(claim crawlcampaign.PooledClaim) map[string]any {
	extras := map[string]any{
		"_fresh_lead_run_id":      claim.Fence.RunID,
		"_fresh_lead_attempt":     claim.Fence.Attempt,
		"_fresh_lead_campaign_id": claim.CampaignID,
		"_fresh_lead_source_id":   claim.SourceID,
	}
	if !claim.FreshCutoffAt.IsZero() {
		extras["_fresh_lead_cutoff_at"] = claim.FreshCutoffAt.UTC().Format(time.RFC3339)
	}
	return extras
}

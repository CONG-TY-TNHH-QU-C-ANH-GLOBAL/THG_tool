package leads

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// Auto-archive sweep (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-3). Org-scoped
// maintenance: project each live lead's lifecycle + coverage, ask the pure
// models.EvaluateArchive whether it should be retired, and flip archived_at for the ones
// that should. No hard delete — the engagement ledger is never touched, so dedup +
// multi-actor coverage history still see archived leads.

// archiveSweepBatch caps how many leads one sweep evaluates per org per tick, so a large
// tenant cannot stall the scheduler. The next tick continues where this left off (newly
// archived leads drop out of the candidate set).
const archiveSweepBatch = 500

// ArchiveSweep evaluates an org's live leads and archives those past policy. It is the
// testable core the periodic scheduler calls per org.
func (s *Store) ArchiveSweep(ctx context.Context, orgID int64, policy models.LeadLifecyclePolicy, coveragePolicy models.CoveragePolicy, website string) (models.ArchiveSweepReport, error) {
	report := models.ArchiveSweepReport{ByReason: map[string]int{}}
	if orgID <= 0 {
		return report, fmt.Errorf("archive sweep requires org_id")
	}
	// Sweep every live state (archived is already excluded upstream; stale opted in).
	items, err := s.GetWorkQueue(ctx, orgID, models.WorkQueueOptions{
		States:          []models.LeadFreshnessState{models.LeadActive, models.LeadWaitingReply, models.LeadFollowupDue},
		IncludeStale:    true,
		ComputeCoverage: true,
		Website:         website,
		Policy:          policy,
		CoveragePolicy:  coveragePolicy,
		Limit:           archiveSweepBatch,
	})
	if err != nil {
		return report, err
	}
	now := time.Now()
	for _, it := range items {
		report.Scanned++
		in := models.ArchiveInputs{
			Lifecycle:     it.Lifecycle,
			Coverage:      it.Coverage,
			CoverageFull:  coveragePolicy.MaxAccountsPerLead > 0 && it.Coverage.OrgTouchCount >= coveragePolicy.MaxAccountsPerLead,
			InvalidTarget: candidateInvalidTarget(it.Lead),
		}
		ok, reason := models.EvaluateArchive(in, policy, now)
		if !ok {
			continue
		}
		if err := s.ArchiveLead(ctx, orgID, it.Lead.ID, reason); err != nil {
			continue // best-effort: a single failed archive must not abort the sweep
		}
		report.Archived++
		report.ByReason[reason]++
	}
	return report, nil
}

// candidateInvalidTarget reports a lead with no actionable target at all. Conservative by
// design — richer URL-shape validation lives in the planner's resolveOutboundTargetURL;
// the sweep only auto-archives the clearly-unactionable to avoid false retirement.
func candidateInvalidTarget(lead models.Lead) bool {
	return strings.TrimSpace(lead.SourceURL) == "" && strings.TrimSpace(lead.AuthorURL) == ""
}

// OrgIDsWithActiveLeads returns the distinct orgs that still own non-archived leads — the
// work list the periodic auto-archive scheduler iterates.
func (s *Store) OrgIDsWithActiveLeads(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT COALESCE(org_id,0) FROM leads
		  WHERE archived_at IS NULL AND COALESCE(org_id,0) > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orgIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		orgIDs = append(orgIDs, id)
	}
	return orgIDs, rows.Err()
}

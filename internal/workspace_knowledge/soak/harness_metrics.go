package soak

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// Soak scoring + measurement: fallback detection, hit-signal scan, verdict, replay
// health + staleness. Split from harness.go; same package, behavior unchanged.

// detectFallback returns the first fallback_primary_* reason recorded in the trace,
// or "" if the primary searcher served the query.
func detectFallback(trace retrieval.Trace) string {
	for r := range trace.TotalByReason {
		if strings.HasPrefix(string(r), "fallback_primary_") {
			return string(r)
		}
	}
	return ""
}

// scanHitSignals collects the retrieved titles plus the non-negotiable compliance/hidden
// leak lists (banned-claim / hidden assets must never surface). Verbatim extraction.
func scanHitSignals(hits []retrieval.Hit) (titles, complianceLeaks, hiddenLeaks []string) {
	for _, hit := range hits {
		if hit.Asset == nil {
			continue
		}
		titles = append(titles, hit.Asset.Title)
		// SAFETY CHECKS — these are the non-negotiables.
		if hit.Asset.Type == assets.AssetBannedClaim {
			complianceLeaks = append(complianceLeaks, hit.Asset.Title)
		}
		if hit.Asset.State == assets.StateHidden {
			hiddenLeaks = append(hiddenLeaks, hit.Asset.Title)
		}
	}
	return titles, complianceLeaks, hiddenLeaks
}

// soakVerdict applies the precision-dominant verdict policy. PRECISION is the dominant
// quality signal, NOT raw score: different searchers produce wildly different score scales
// (hybrid in [0,1], RRF in [0,~0.05] from 1/(60+rank)), so cross-scale comparison would be
// apples-to-oranges. BelowMinScore stays as report observability metadata, not a gate.
//
//	FAIL      any compliance / hidden leak (non-negotiable)
//	FAIL      expected intent + hits returned but precision == 0 (wrong stuff surfaced)
//	DEGRADED  expected intent + no hits (orchestrator should ask for clarification)
//	DEGRADED  precision < 0.4 (weak relevance)
//	PASS      otherwise — including "no intent → no hits expected"
func soakVerdict(out PromptOutcome, intentTags []string, hitCount int) string {
	switch {
	case len(out.ComplianceLeaks) > 0 || len(out.HiddenLeaks) > 0:
		return "FAIL"
	case len(intentTags) > 0 && hitCount > 0 && out.PrecisionAtK == 0:
		return "FAIL"
	case len(intentTags) > 0 && hitCount == 0:
		return "DEGRADED"
	case len(intentTags) > 0 && out.PrecisionAtK < 0.4:
		return "DEGRADED"
	default:
		return "PASS"
	}
}

// measureReplayHealth: verify every recent retrieval event has a
// well-formed trace. Reads from knowledge_events directly via the
// existing ListKnowledgeReplayEventsForOrg path.
func (h *Harness) measureReplayHealth(ctx context.Context) ReplayHealth {
	rh := ReplayHealth{}
	events, err := h.Store.Knowledge().ListReplayEventsForOrg(ctx, h.OrgID, "", 100)
	if err != nil {
		return rh
	}
	for _, ev := range events {
		rh.TracesProduced++
		var parsed retrieval.Trace
		if len(ev.Trace) > 0 {
			_ = json.Unmarshal(ev.Trace, &parsed)
		}
		complete := true
		if parsed.SearcherImpl == "" {
			rh.MissingSearcherImpl++
			complete = false
		}
		if len(parsed.Selected) == 0 && parsed.CandidatesConsidered > 0 {
			rh.MissingSelected++
			complete = false
		}
		if complete {
			rh.TracesComplete++
		}
	}
	if rh.TracesProduced > 0 {
		rh.CompletenessRate = float64(rh.TracesComplete) / float64(rh.TracesProduced)
	}
	return rh
}

// measureStale: stale asset detection using the existing
// CountStaleKnowledgeAssetsForOrg query.
func (h *Harness) measureStale(ctx context.Context) StaleMetrics {
	s := StaleMetrics{
		TotalAssets: len(h.Catalog),
	}
	if stale, err := h.Store.Knowledge().CountStaleAssetsForOrg(ctx, h.OrgID, 30); err == nil {
		s.StalePast30d = stale
	}
	// Never-retrieved vs. fresh: derive from Stats (which counts
	// retrieval_count_30d > 0 vs == 0).
	if ks, err := h.Store.Knowledge().GetStatsForOrg(ctx, h.OrgID); err == nil {
		s.NeverRetrieved = ks.TotalAssets - len(ks.TopRetrieved)
	}
	return s
}

// isTraceComplete is the schema check the soak applies to every
// trace it observes. Goal directive PR-2 §3 — additive-compatible
// means OLD events with missing fields are tolerated, but NEW
// events MUST be complete.
func isTraceComplete(t retrieval.Trace) bool {
	if t.SearcherImpl == "" {
		return false
	}
	// CandidatesConsidered may be 0 for empty-result queries; not a
	// completeness signal.
	return true
}

// Package hybrid implements [retrieval.Searcher] with the next layer
// of ranking signals beyond the naive token-overlap baseline:
//
//   - metadata filter (state, type, tags) — same as naive (delegated
//     to the store).
//   - keyword scoring — token overlap with EXACT-PHRASE bonus when
//     the full query phrase appears in title or description verbatim.
//   - recency — assets retrieved recently get a small score lift.
//   - governance — banned_claim assets are filtered at the candidate
//     pool level (before scoring) so they never reach the assembled
//     prompt, period. The naive searcher relied on the assembly
//     layer to drop them; this searcher refuses to score them.
//
// Intentionally NOT included (per user direction in roadmap §3):
//   - trigram / BM25 (needs FTS5 or Postgres) — deferred to Phase C.2
//   - pgvector semantic — explicitly LAST; user said "đừng vội"
//
// Score formula:
//
//   text     = max(token_overlap, exact_phrase_bonus)        [0, 1]
//   recency  = 1.0 if last_retrieved_at within 7d, else 0    [0, 0.10]
//   governance: banned_claim → REJECT (no score, no inclusion)
//
//   final = clamp01(0.50*text + 0.20*boost + 0.20*pin + 0.10*recency)
//
// The pin weight dropped from 0.25 (naive) to 0.20 here. Reason: with
// recency contributing, pin no longer needs to single-handedly surface
// CTAs — a pinned CTA that was actually retrieved recently scores
// 0.20 + 0.10 = 0.30, higher than naive's 0.25.
//
// Score breakdown still surfaces every component to the trace so
// the Replay UI can explain why each asset surfaced.
package hybrid

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// SearcherImpl identifies traces produced by this searcher.
const SearcherImpl = "hybrid-v1"

// AssetLister is the narrow store surface the searcher needs.
type AssetLister interface {
	ListAssetsForOrg(ctx context.Context, orgID int64, filter assets.ListFilter) ([]*assets.Asset, error)
}

// Searcher implements retrieval.Searcher.
type Searcher struct {
	Lister AssetLister
	// Clock is overridable for deterministic test runs. Defaults to time.Now.
	Clock func() time.Time
}

// New constructs a hybrid searcher.
func New(lister AssetLister) *Searcher {
	return &Searcher{Lister: lister, Clock: time.Now}
}

func (s *Searcher) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// TopK satisfies retrieval.Searcher.
func (s *Searcher) TopK(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, error) {
	hits, _, err := s.TopKWithTrace(ctx, orgID, query, filter, k)
	return hits, err
}

// TopKWithTrace runs the search with full explainability.
func (s *Searcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	trace := retrieval.Trace{
		Query:         retrieval.TruncateQuery(query),
		SearcherImpl:  SearcherImpl,
		TotalByReason: map[retrieval.RejectionReason]int{},
	}
	if s.Lister == nil {
		return nil, trace, fmt.Errorf("hybrid searcher: lister is nil")
	}
	if orgID <= 0 {
		return nil, trace, fmt.Errorf("hybrid searcher: org_id required")
	}
	if k <= 0 {
		return nil, trace, nil
	}

	queryTokens := retrieval.Tokenize(query)
	queryPhrase := strings.ToLower(strings.TrimSpace(query))

	listFilter := assets.ListFilter{
		Types:   filter.Types,
		States:  filter.EffectiveStates(),
		OrderBy: assets.OrderDefault,
		Limit:   candidateLimit(k),
		// Retrieval hot path: never quote from stale/unhealthy catalogs.
		ExcludeUnhealthySources: true,
	}
	candidates, err := s.Lister.ListAssetsForOrg(ctx, orgID, listFilter)
	if err != nil {
		return nil, trace, err
	}
	trace.CandidatesConsidered = len(candidates)

	now := s.now()
	type scored struct {
		asset     *assets.Asset
		score     float64
		breakdown retrieval.ScoreBreakdown
		reason    string
	}
	scoredHits := make([]scored, 0, len(candidates))
	for _, a := range candidates {
		// Governance: banned claims do not score, ever. This is the
		// signal that this searcher is doing more than naive.
		if a.Type == assets.AssetBannedClaim {
			retrieval.RecordRejection(&trace, a, retrieval.RejectGovernance, 0)
			continue
		}
		if !filterByTags(a, filter.Tags) {
			retrieval.RecordRejection(&trace, a, retrieval.RejectTagFilter, 0)
			continue
		}
		score, breakdown, reason := scoreAsset(a, queryTokens, queryPhrase, now)
		if score <= 0 {
			retrieval.RecordRejection(&trace, a, retrieval.RejectBelowThreshold, score)
			continue
		}
		scoredHits = append(scoredHits, scored{a, score, breakdown, reason})
	}

	sort.SliceStable(scoredHits, func(i, j int) bool {
		return scoredHits[i].score > scoredHits[j].score
	})

	keep := scoredHits
	if len(keep) > k {
		for _, sh := range scoredHits[k:] {
			retrieval.RecordRejection(&trace, sh.asset, retrieval.RejectTopKCap, sh.score)
		}
		keep = scoredHits[:k]
	}

	hits := make([]retrieval.Hit, 0, len(keep))
	trace.Selected = make([]retrieval.ScoredHit, 0, len(keep))
	for _, sh := range keep {
		hits = append(hits, retrieval.Hit{Asset: sh.asset, Score: sh.score, Reason: sh.reason})
		trace.Selected = append(trace.Selected, retrieval.ScoredHit{
			AssetID:   sh.asset.ID,
			Title:     sh.asset.Title,
			Type:      sh.asset.Type,
			Score:     sh.score,
			Breakdown: sh.breakdown,
			Reason:    sh.reason,
		})
	}
	return hits, trace, nil
}

// scoreAsset computes the final score + breakdown + reason label.
// Documented inline since this is THE place tuning happens.
func scoreAsset(a *assets.Asset, queryTokens map[string]struct{}, queryPhrase string, now time.Time) (float64, retrieval.ScoreBreakdown, string) {
	// Text-match component: max(token-overlap, exact-phrase bonus).
	// Exact phrase in title or description gets the full text-match
	// share (0.50) regardless of token count — operators expect
	// "best price guaranteed POD" to surface assets that literally
	// say that phrase, not just assets that share two tokens.
	assetTokens := retrieval.Tokenize(a.Title)
	for _, t := range a.Tags {
		for tok := range retrieval.Tokenize(t) {
			assetTokens[tok] = struct{}{}
		}
	}
	overlap := 0
	for tok := range queryTokens {
		if _, ok := assetTokens[tok]; ok {
			overlap++
		}
	}
	qSize := max(len(queryTokens), 1)
	overlapFrac := float64(overlap) / float64(qSize)

	// Title-phrase match is the strongest text signal (full bonus);
	// description-phrase match is a softer signal (half bonus). Both
	// the title and the description matter, but title is canonical —
	// "POD shirt fulfillment" titled asset must outrank "we sell POD
	// shirts" described asset for the query "pod shirt".
	phraseBonus := 0.0
	if queryPhrase != "" {
		if strings.Contains(strings.ToLower(a.Title), queryPhrase) {
			phraseBonus = 1.0
		} else if strings.Contains(strings.ToLower(a.Description), queryPhrase) {
			phraseBonus = 0.5
		}
	}
	textFrac := math.Max(overlapFrac, phraseBonus)

	// Recency component: 1.0 if retrieved within 7 days, decaying
	// linearly to 0 at 30 days. Never-retrieved assets get 0 —
	// recency should not penalise fresh assets (boost/pin signal
	// surfaces them).
	recencyFrac := 0.0
	if a.Metrics.LastRetrievedAt != nil {
		age := now.Sub(*a.Metrics.LastRetrievedAt)
		switch {
		case age < 7*24*time.Hour:
			recencyFrac = 1.0
		case age < 30*24*time.Hour:
			// 7d → 1.0, 30d → 0.0 linear.
			recencyFrac = 1.0 - float64(age-7*24*time.Hour)/float64(23*24*time.Hour)
		}
	}

	bd := retrieval.ScoreBreakdown{
		TextMatch: 0.50 * textFrac,
		Boost:     0.20 * float64(a.Boost) / 100.0,
		Recency:   0.10 * recencyFrac,
	}
	if a.Pinned {
		bd.Pin = 0.20
	}
	score := retrieval.Clamp01(bd.TextMatch + bd.Boost + bd.Pin + bd.Recency)

	reason := buildReason(overlap, phraseBonus > 0, recencyFrac > 0, a.Pinned, a.Boost)
	return score, bd, reason
}

func buildReason(overlap int, hasPhrase, hasRecency, pinned bool, boost int) string {
	parts := []string{}
	if hasPhrase {
		parts = append(parts, "exact-phrase")
	} else if overlap > 0 {
		parts = append(parts, fmt.Sprintf("token-match=%d", overlap))
	}
	if pinned {
		parts = append(parts, "pinned")
	}
	if hasRecency {
		parts = append(parts, "recent")
	}
	if boost > 0 {
		parts = append(parts, fmt.Sprintf("boost=%d", boost))
	}
	if len(parts) == 0 {
		return "no-signals"
	}
	return strings.Join(parts, ", ")
}

// Shared helpers (tokenize, truncateQuery, recordRejection, clamp01)
// moved to retrieval/helpers.go — hybrid now uses those public
// versions so naive + hybrid + pgvector share one source of truth.
// buildReason stays local because hybrid has more signals to surface
// (exact-phrase, recency) than the generic helper covers.

func candidateLimit(k int) int {
	return min(max(k*10, 50), 500)
}

func filterByTags(a *assets.Asset, want []string) bool {
	if len(want) == 0 {
		return true
	}
	have := make(map[string]struct{}, len(a.Tags))
	for _, t := range a.Tags {
		have[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}
	for _, w := range want {
		if _, ok := have[strings.ToLower(strings.TrimSpace(w))]; ok {
			return true
		}
	}
	return false
}

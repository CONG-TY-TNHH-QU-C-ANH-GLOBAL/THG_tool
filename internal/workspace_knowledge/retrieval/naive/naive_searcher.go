// Package naive implements [retrieval.Searcher] using token-overlap
// scoring on top of the SQL store. It is the MVP retrieval backend:
// good enough for catalogs up to a few thousand assets per workspace,
// honest about its limitations, and easy to replace with pgvector or
// a hosted embedding service later (same Searcher contract).
//
// Scoring formula (documented so future engineers understand the
// tradeoffs we accepted):
//
//   tokens(q)   = lowercased word-set extracted from the query text
//   tokens(a)   = lowercased word-set from title + tags
//   overlap(a)  = |tokens(q) ∩ tokens(a)|
//   base(a)     = overlap(a) / max(1, |tokens(q)|)         in [0, 1]
//   boost(a)    = a.Boost / 100                             in [0, 1]
//   pin(a)      = 0.25 if a.Pinned else 0                   constant
//   score(a)    = clamp01(0.55*base + 0.20*boost + 0.25*pin)
//
// Pinned assets ALWAYS win ties because their pin contribution is a
// fixed quarter of the total. That matches the design-doc invariant:
// operators who pin an item expect retrieval to honour the pin.
//
// Limitations the team should know about:
//   - No stemming, no synonyms ("dog" ≠ "doggy"). The retrieval
//     engine cannot bridge the language gap a real model would.
//   - No semantic similarity ("cat tee" matches "cat mug" only
//     because both contain "cat"). Phase C.2 swaps in pgvector to fix.
//   - Tag matching is a substring against the tags column, which
//     means "cat" matches "category" too. Acceptable for the MVP
//     since pin/boost lets operators override; documented because
//     it WILL bite us if anyone naive-search-tunes against real data.
package naive

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// AssetLister is the narrow store surface the searcher needs.
// *store.Store satisfies this; tests provide a fake.
type AssetLister interface {
	ListAssetsForOrg(ctx context.Context, orgID int64, filter assets.ListFilter) ([]*assets.Asset, error)
}

// Searcher implements retrieval.Searcher.
type Searcher struct {
	Lister AssetLister
}

// New returns a Searcher backed by lister.
func New(lister AssetLister) *Searcher {
	return &Searcher{Lister: lister}
}

// SearcherImpl is the trace.searcher_impl tag for this implementation.
// Stable string used by the Operator Replay UI to identify which
// searcher produced an event.
const SearcherImpl = "naive-v1"

// TopK satisfies retrieval.Searcher. Delegates to TopKWithTrace and
// discards the trace; callers on the simple path pay the same compute
// cost but skip the bookkeeping overhead — fine for the MVP given the
// catalog scale here.
func (s *Searcher) TopK(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, error) {
	hits, _, err := s.TopKWithTrace(ctx, orgID, query, filter, k)
	return hits, err
}

// TopKWithTrace runs the search and produces an explainability Trace
// in one pass. See [retrieval.Trace] for the shape and rules.
func (s *Searcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	trace := retrieval.Trace{
		Query:         retrieval.TruncateQuery(query),
		SearcherImpl:  SearcherImpl,
		TotalByReason: map[retrieval.RejectionReason]int{},
	}
	if s.Lister == nil {
		return nil, trace, fmt.Errorf("naive searcher: lister is nil")
	}
	if orgID <= 0 {
		return nil, trace, fmt.Errorf("naive searcher: org_id required")
	}
	if k <= 0 {
		return nil, trace, nil
	}

	queryTokens := retrieval.Tokenize(query)
	listFilter := assets.ListFilter{
		Types:   filter.Types,
		States:  filter.EffectiveStates(),
		OrderBy: assets.OrderDefault,
		Limit:   candidateLimit(k),
	}
	candidates, err := s.Lister.ListAssetsForOrg(ctx, orgID, listFilter)
	if err != nil {
		return nil, trace, err
	}
	trace.CandidatesConsidered = len(candidates)

	// Phase 1: score everything, collect tag-filter rejections.
	type scored struct {
		asset     *assets.Asset
		score     float64
		breakdown retrieval.ScoreBreakdown
		reason    string
	}
	scoredHits := make([]scored, 0, len(candidates))
	for _, a := range candidates {
		if !filterByTags(a, filter.Tags) {
			retrieval.RecordRejection(&trace, a, retrieval.RejectTagFilter, 0)
			continue
		}
		score, breakdown, reason := scoreAssetWithBreakdown(a, queryTokens)
		if score <= 0 {
			retrieval.RecordRejection(&trace, a, retrieval.RejectBelowThreshold, score)
			continue
		}
		scoredHits = append(scoredHits, scored{a, score, breakdown, reason})
	}

	sort.SliceStable(scoredHits, func(i, j int) bool {
		return scoredHits[i].score > scoredHits[j].score
	})

	// Phase 2: cap to k. Rows that scored above threshold but lost the
	// cut are reported as RejectTopKCap so the operator sees "we
	// considered N viable assets but k=6 limited it" rather than
	// guessing.
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

// candidateLimit is the SQL LIMIT we apply when pulling candidates
// from the store. Larger than k because in-memory ranking may demote
// SQL's pinned-first order — we want enough headroom for that without
// dragging the entire catalog into memory.
func candidateLimit(k int) int {
	n := min(max(k*10, 50), 500)
	return n
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

// scoreAssetWithBreakdown is the explainability-aware scoring path.
// Returns the same final score as the legacy scoreAsset would but
// also exposes the per-signal contributions so the replay UI can
// render a stacked bar of "this is 60% text-match, 25% pin, 15% boost".
//
// The Recency field stays 0 in the naive searcher — that signal lives
// in the hybrid implementation.
func scoreAssetWithBreakdown(a *assets.Asset, queryTokens map[string]struct{}) (float64, retrieval.ScoreBreakdown, string) {
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
	baseFrac := float64(overlap) / float64(qSize)
	bd := retrieval.ScoreBreakdown{
		TextMatch: 0.55 * baseFrac,
		Boost:     0.20 * float64(a.Boost) / 100.0,
	}
	if a.Pinned {
		bd.Pin = 0.25
	}
	score := retrieval.Clamp01(bd.TextMatch + bd.Boost + bd.Pin + bd.Recency)

	reason := ""
	switch {
	case overlap > 0 && a.Pinned:
		reason = fmt.Sprintf("title-or-tag-match=%d, pinned, boost=%d", overlap, a.Boost)
	case overlap > 0:
		reason = fmt.Sprintf("title-or-tag-match=%d, boost=%d", overlap, a.Boost)
	case a.Pinned:
		reason = "pinned"
	default:
		reason = fmt.Sprintf("boost=%d", a.Boost)
	}
	return score, bd, reason
}

// All shared helpers (tokenize, truncateQuery, recordRejection,
// clamp01) moved to retrieval/helpers.go — naive now calls those
// public versions to avoid drift with hybrid + pgvector.

package retrieval

import (
	"fmt"
	"math"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Shared helpers for Searcher implementations. Each implementation
// previously had its own copies (naive, hybrid). When pgvector landed
// as a third implementation it would have triplicated the same
// 30-line helpers — so they moved here.
//
// Keep these PURE. Anything that needs Searcher state goes on the
// Searcher struct, not in a helper.

// TruncateQuery caps a user-supplied query at the storage limit.
// Length is the SAME in every searcher's trace.Query field so the
// Replay UI never sees "embedding hit but query column truncated
// in retrieval-X format only" surprises.
const maxQueryStored = 240

func TruncateQuery(q string) string {
	if len(q) <= maxQueryStored {
		return q
	}
	return q[:maxQueryStored] + "…"
}

// RecordRejection adds one entry to the trace's Rejected list AND
// bumps TotalByReason. Capacity is bounded so a 10k-row catalog
// with all rejections doesn't blow up the events table — only the
// first sampleCapPerReason of each reason embed in Rejected[],
// while TotalByReason keeps the uncapped histogram.
const SampleCapPerReason = 5

func RecordRejection(t *Trace, a *assets.Asset, reason RejectionReason, score float64) {
	if t == nil || a == nil {
		return
	}
	if t.TotalByReason == nil {
		t.TotalByReason = map[RejectionReason]int{}
	}
	t.TotalByReason[reason]++
	if t.TotalByReason[reason] > SampleCapPerReason {
		return
	}
	t.Rejected = append(t.Rejected, RejectedCandidate{
		AssetID: a.ID,
		Title:   a.Title,
		Type:    a.Type,
		Reason:  reason,
		Score:   score,
	})
}

// Clamp01 keeps a score in [0, 1]. NaN maps to 0 (defensive — a
// NaN score would sort unpredictably and break ranking determinism).
func Clamp01(x float64) float64 {
	if math.IsNaN(x) || x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// Tokenize is the shared word-set extractor for lexical searchers.
// Lower-cases, splits on non-alphanumeric, drops tokens shorter than
// 2 chars. Returns a set so callers can intersect cheaply.
//
// Deliberately NOT removing stopwords — short search queries
// ("cat tee POD") lose more signal than they gain from stopword
// removal. If a future searcher wants stemming, add a new helper
// rather than mutating this — score semantics across naive + hybrid
// MUST stay stable so the Operator Replay surface remains
// reproducible.
func Tokenize(s string) map[string]struct{} {
	out := map[string]struct{}{}
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() >= 2 {
			out[cur.String()] = struct{}{}
		}
		cur.Reset()
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// BuildReason composes the operator-facing label that explains why
// an asset surfaced. Used by both pgvector and (in PR-3) RRF — the
// hybrid searcher has its own variant because it has more signals
// to mention. Kept in retrieval package so the format stays stable
// across implementations.
func BuildReason(semanticSimilarity float64, pinned bool, boost int) string {
	parts := []string{}
	if semanticSimilarity > 0 {
		parts = append(parts, fmt.Sprintf("semantic=%.2f", semanticSimilarity))
	}
	if pinned {
		parts = append(parts, "pinned")
	}
	if boost > 0 {
		parts = append(parts, fmt.Sprintf("boost=%d", boost))
	}
	if len(parts) == 0 {
		return "no-signals"
	}
	return strings.Join(parts, ", ")
}

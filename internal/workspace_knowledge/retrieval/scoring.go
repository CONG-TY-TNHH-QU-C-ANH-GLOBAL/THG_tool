package retrieval

import (
	"fmt"
	"math"
	"strings"
)

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

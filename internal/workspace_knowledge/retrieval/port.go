// Package retrieval defines the Layer-4 port for the Workspace
// Knowledge OS. A Searcher returns the top-K assets that best match a
// query for a given tenant. Implementations range from a SQL LIKE
// ranker (MVP) to pgvector / Qdrant / Weaviate (production) — the
// contract is identical.
//
// This package contains the CONTRACT only. Concrete searchers live in
// sibling packages and land in Phase C of the roadmap. See
// [specs/WORKSPACE_KNOWLEDGE_OS.md §8].
//
// Why a port: the choice of retrieval backend is an infrastructure
// decision that the agent runtime should not care about. As long as
// the Searcher satisfies this interface, swapping implementations is
// invisible to the comment-generator code path.
package retrieval

import (
	"context"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Searcher returns the top-K assets that best match a query.
type Searcher interface {
	// TopK runs the search for orgID with the given filter and returns
	// up to k Hits, sorted by descending Score. An empty result is
	// not an error.
	//
	// Implementations MUST honor [retrieval.SearchFilter.States] —
	// the retrieval engine never returns hidden or pending assets
	// unless the caller explicitly asks for them (default = approved
	// only). This is invariant #4 in the design doc.
	TopK(ctx context.Context, orgID int64, query string, filter SearchFilter, k int) ([]Hit, error)

	// TopKWithTrace is the explainability variant. It returns the
	// same hits as TopK PLUS a [Trace] describing every decision
	// the searcher made — candidates considered, candidates rejected
	// with reasons, score breakdown per hit, searcher impl identity.
	//
	// The runtime calls this on the explainability hot path (per
	// lead) and TopK on simple-batch use cases. Implementations
	// SHOULD share the inner ranking code; only the bookkeeping
	// differs between the two.
	TopKWithTrace(ctx context.Context, orgID int64, query string, filter SearchFilter, k int) ([]Hit, Trace, error)
}

// SearchFilter narrows a query. Empty States means "approved only" at
// the Searcher implementation level. Empty Types means "any type."
type SearchFilter struct {
	Types  []assets.AssetType
	States []assets.AssetState
	// Tags optionally restricts to assets that contain at least one of
	// the listed tags. Matches the AND-of-ORs convention common in
	// retrieval systems: at least one tag must match.
	Tags []string
}

// EffectiveStates returns States if the caller specified any,
// otherwise the default of {approved}. Used by every Searcher
// implementation as a one-liner — keeps the "approved-only by
// default" invariant in exactly one place instead of being repeated
// in each searcher's WHERE clause.
func (f SearchFilter) EffectiveStates() []assets.AssetState {
	if len(f.States) == 0 {
		return []assets.AssetState{assets.StateApproved}
	}
	return f.States
}

// Hit is one result from a Searcher.
type Hit struct {
	// Asset is the full asset row. Always non-nil for successful hits.
	Asset *assets.Asset

	// Score is the relevance score in [0, 1]. The exact semantics are
	// implementation-defined — naive LIKE ranking might use
	// match-count / token-count, pgvector uses 1 - cosine distance.
	// The score field is comparable WITHIN ONE TopK call (used for
	// ordering) but not necessarily across implementations.
	Score float64

	// Reason is a short human label describing why this asset
	// matched: "title-match", "tag-match", "semantic-cosine", etc.
	// Surfaced in the Operator Replay UI so the operator can audit
	// retrieval decisions. Required, not optional — "no reason
	// available" is unacceptable for an opaque retrieval system.
	Reason string
}

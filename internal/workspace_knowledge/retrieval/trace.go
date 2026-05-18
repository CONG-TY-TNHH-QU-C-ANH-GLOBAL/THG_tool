package retrieval

import (
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Trace is the explainability record produced by one TopK call.
// It is what powers the Operator Replay surface in the UI — every
// decision the searcher made gets a row here so the operator can
// answer "why was THIS asset retrieved and that one rejected?"
//
// A Trace travels with the search result, then through assembly
// (which adds budget-trimming info) and finally into Layer-7 metrics
// storage. The shape is intentionally stable so dashboards built
// against historical events continue to work as the searcher
// implementation evolves.
//
// Three rules for adding fields:
//   1. Default values are valid — older events without the field
//      must still render in the UI. Never make a field required by
//      consumers.
//   2. Field names match operator vocabulary (UI labels), not
//      engineer vocabulary. "candidates_considered" not "pool_size".
//   3. Numbers and IDs only — no LLM outputs, no PII. The Trace is
//      INDEX-friendly; the prompt itself lives elsewhere.
type Trace struct {
	// Query is the lead-content snippet the searcher tokenized.
	// Truncated to a few hundred chars at the storage boundary.
	Query string `json:"query"`

	// CandidatesConsidered is the size of the pool BEFORE filtering /
	// ranking. The retrieval engine pulls a generous pool then
	// re-ranks — this number gives operators a sense of catalog scale.
	CandidatesConsidered int `json:"candidates_considered"`

	// Selected is the list of Hits that made it into the final TopK.
	// Score breakdown explains the score; reason is a short human label.
	Selected []ScoredHit `json:"selected"`

	// Rejected enumerates assets the engine looked at but did NOT
	// return. The reason taxonomy is enumerated in RejectionReason
	// below — every entry must carry one of those values.
	//
	// Capacity-bounded: only the first N rejections per reason flow
	// through (the engine truncates to keep traces small). The
	// counter Rejected.TotalByReason holds the full counts.
	Rejected []RejectedCandidate `json:"rejected,omitempty"`

	// TotalByReason is the uncapped histogram of rejection reasons,
	// so the UI can say "12 hidden, 4 wrong type" even when only the
	// first 3 of each were embedded in Rejected[] above.
	TotalByReason map[RejectionReason]int `json:"total_by_reason,omitempty"`

	// SearcherImpl identifies which Searcher produced this trace —
	// "naive-v1", "hybrid-v1", "pgvector-v1". Lets the replay UI
	// disambiguate when the team rolls out a new searcher and old
	// + new events coexist in the dashboard.
	SearcherImpl string `json:"searcher_impl"`
}

// ScoredHit is the explainability-enriched counterpart to Hit. The
// Score field is the same as Hit.Score; Breakdown is what the
// replay surface uses to draw the score-composition bar.
type ScoredHit struct {
	AssetID   int64          `json:"asset_id"`
	Title     string         `json:"title"`
	Type      assets.AssetType `json:"type"`
	Score     float64        `json:"score"`
	Breakdown ScoreBreakdown `json:"breakdown"`
	Reason    string         `json:"reason"`
	// RRF-specific fields. Zero / omitted on non-RRF searchers.
	// Goal directive PR-3 §6 — explainability moat. The Replay UI
	// shows "this asset ranked #2 in BM25 and #8 in semantic, RRF
	// score 0.031" so operators can audit why fusion picked it.
	BM25Rank     int     `json:"bm25_rank,omitempty"`
	SemanticRank int     `json:"semantic_rank,omitempty"`
	RRFScore     float64 `json:"rrf_score,omitempty"`
}

// ScoreBreakdown decomposes the final Score into its contributing
// signals. The fields sum to approximately Score after clamping
// (clamp01 may trim the sum if it exceeds 1.0). Operators read this
// to answer "is this asset surfacing because of text match or
// because someone pinned it?".
type ScoreBreakdown struct {
	// TextMatch is the lexical-similarity contribution. For the naive
	// searcher this is `0.55 * overlap_fraction`. For hybrid it
	// includes keyword + (eventual) trigram. Never negative.
	TextMatch float64 `json:"text_match"`
	// Boost is the operator-controlled rank lift. `0.20 * (boost/100)`.
	Boost float64 `json:"boost"`
	// Pin is the pinned-flag contribution. Currently fixed at 0.25
	// when pinned, 0 otherwise. The 25% share is documented in the
	// naive searcher — same number applies here for stability.
	Pin float64 `json:"pin"`
	// Recency is the hybrid-searcher-only freshness contribution.
	// 0 in the naive searcher. Documented in the hybrid package.
	Recency float64 `json:"recency"`
	// Semantic is the pgvector-only contribution. 0 in lexical
	// searchers (naive, hybrid). For pgvector this is the dominant
	// signal: `0.70 * cosine_similarity`. Older Replay events
	// without this field unmarshal to 0.0 — additive-compatible by
	// design (goal directive PR-2 §3).
	Semantic float64 `json:"semantic,omitempty"`
}

// RejectionReason is the closed enum the UI groups rejections by.
// Each reason MUST be self-explanatory in the replay row tooltip;
// add a new reason only when an existing one would mislead the
// operator into thinking a different layer is at fault.
type RejectionReason string

const (
	// RejectStateFilter — asset state did not match SearchFilter.States
	// (e.g. hidden / pending when the runtime asked approved-only).
	RejectStateFilter RejectionReason = "state_filter"
	// RejectTypeFilter — asset type did not match SearchFilter.Types.
	RejectTypeFilter RejectionReason = "type_filter"
	// RejectTagFilter — asset did not carry any requested tag.
	RejectTagFilter RejectionReason = "tag_filter"
	// RejectBelowThreshold — score did not exceed the minimum cutoff.
	RejectBelowThreshold RejectionReason = "below_threshold"
	// RejectGovernance — banned_claim assets filtered before they can
	// even score. This reason will surface on the hybrid searcher;
	// the naive searcher does not produce it (no governance layer).
	RejectGovernance RejectionReason = "governance_drop"
	// RejectTopKCap — asset scored above threshold but lost the cut
	// because k was already filled by higher-scored assets. The most
	// common reason for a non-pinned-non-boosted asset to be absent.
	RejectTopKCap RejectionReason = "topk_cap"
	// RejectSemanticThreshold — vector similarity below the
	// confidence cutoff. Emitted by the pgvector Searcher only.
	// Distinct from RejectBelowThreshold (lexical) so the Replay UI
	// can distinguish "weak keyword match" from "weak semantic match"
	// — operators tune these via different levers (boost vs.
	// re-embed).
	RejectSemanticThreshold RejectionReason = "semantic_threshold"
	// RejectEmbeddingMissing — asset is in catalog but its vector
	// has not been generated yet (status='pending'), failed, or was
	// generated under a different model. The Searcher skips these
	// to preserve query consistency; the embedding worker is
	// expected to backfill them. NOT a quality signal — purely a
	// readiness signal.
	RejectEmbeddingMissing RejectionReason = "embedding_missing"
)

// RejectedCandidate is one row of the rejected-pool slice.
type RejectedCandidate struct {
	AssetID int64           `json:"asset_id"`
	Title   string          `json:"title"`
	Type    assets.AssetType `json:"type"`
	Reason  RejectionReason `json:"reason"`
	Score   float64         `json:"score,omitempty"` // 0 when no scoring happened (filtered before scoring)
}

// VectorFilter is the narrow filter passed to store-level vector
// queries. Lives in the retrieval package (not store) so both the
// store and the pgvector Searcher reference the SAME named type —
// without it, the Searcher's VectorStore interface and the store's
// QueryNearestVectors method would have type-name mismatch.
type VectorFilter struct {
	Types  []assets.AssetType
	States []assets.AssetState
}

// VectorHit is one row returned by the store's vector query. Same
// reason as VectorFilter — shared type, neutral home.
type VectorHit struct {
	AssetID  int64
	Distance float64
	Asset    *assets.Asset
}

// AssemblyBudget records what the context-assembly layer did with the
// retrieval result. The replay UI uses this to show "we retrieved 6
// hits but dropped 2 by the MaxProducts cap".
type AssemblyBudget struct {
	AssembledProducts int `json:"assembled_products"`
	AssembledPolicies int `json:"assembled_policies"`
	AssembledCTAs     int `json:"assembled_ctas"`
	DroppedByCap      int `json:"dropped_by_cap"`
	ComplianceDropped int `json:"compliance_dropped"`
	EstimatedTokens   int `json:"estimated_tokens"` // very rough: chars/4
}

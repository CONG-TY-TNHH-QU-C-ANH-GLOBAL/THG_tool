package rrf

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// Goal G6 (BM25/FTS authority): proves the load-bearing invariant
// that RRF is the FINAL authority on ranking while NEVER able to:
//
//   - Override governance (banned_claim assets must not appear).
//   - Override operator pinning (pinned assets must survive even
//     when semantic ranks them last).
//
// The architectural guarantee is structural — RRF receives whatever
// the upstream Lexical / Semantic searchers return, and those
// searchers do the governance + pin work BEFORE RRF runs. This file
// asserts that contract explicitly so any future refactor that
// weakens it fails CI.

// scriptedAuthority is a Searcher whose results we control exactly.
type scriptedAuthority struct {
	hits  []retrieval.Hit
	trace retrieval.Trace
}

func (s *scriptedAuthority) TopK(ctx context.Context, _ int64, _ string, _ retrieval.SearchFilter, _ int) ([]retrieval.Hit, error) {
	return s.hits, nil
}
func (s *scriptedAuthority) TopKWithTrace(ctx context.Context, _ int64, _ string, _ retrieval.SearchFilter, _ int) ([]retrieval.Hit, retrieval.Trace, error) {
	return s.hits, s.trace, nil
}

func authHit(id int64, title string, score float64, typ assets.AssetType, pinned bool) retrieval.Hit {
	return retrieval.Hit{
		Asset: &assets.Asset{
			ID:     id,
			OrgID:  7,
			Type:   typ,
			Title:  title,
			Pinned: pinned,
			State:  assets.StateApproved,
			Payload: json.RawMessage(`{}`),
		},
		Score: score,
	}
}

// Governance invariant: a banned_claim asset MUST NOT appear in RRF
// output even if BOTH upstream searchers (somehow) return it.
//
// This is a defence-in-depth test. The lexical searcher (hybrid)
// already filters banned_claim before producing its ranking, so
// RRF normally never SEES one. But if a future bug let one through,
// THIS test would catch it — we explicitly include a banned_claim
// in BOTH upstream rankings and assert RRF's output filters them.
func TestRRF_NeverSurfacesBannedClaim(t *testing.T) {
	// Both upstreams contain a banned_claim ranked #1.
	lex := &scriptedAuthority{hits: []retrieval.Hit{
		authHit(1, "BANNED: best price guaranteed", 0.9, assets.AssetBannedClaim, false),
		authHit(2, "Cat Tee", 0.5, assets.AssetPODProduct, false),
	}}
	sem := &scriptedAuthority{hits: []retrieval.Hit{
		authHit(1, "BANNED: best price guaranteed", 0.9, assets.AssetBannedClaim, false),
		authHit(2, "Cat Tee", 0.5, assets.AssetPODProduct, false),
	}}
	r := New(lex, sem)

	hits, _, _ := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)

	// CURRENT BEHAVIOUR: RRF does not itself filter banned_claim —
	// it relies on the lex/sem searchers to do that filtering BEFORE
	// passing results. This test documents the contract: if a
	// banned_claim reaches RRF, the bug is upstream, not in RRF.
	// We log here to make the dependency explicit; if we ever want
	// belt-and-braces filtering inside RRF, that's a new feature.
	for _, h := range hits {
		if h.Asset != nil && h.Asset.Type == assets.AssetBannedClaim {
			t.Errorf("BANNED CLAIM SURFACED: id=%d title=%q. "+
				"This is a governance leak — verify upstream searchers (hybrid + pgvector) filter banned_claim BEFORE ranking. "+
				"If they do, but RRF still emitted this, RRF needs a defence-in-depth filter.",
				h.Asset.ID, h.Asset.Title)
		}
	}
}

// Pin invariant: when an operator pins an asset, it MUST survive
// even when the semantic ranking places it last. Goal directive
// PR-3 §4: "Pinned assets MUST survive weak semantic score.
// Operator intent > embedding similarity."
//
// The mechanism: pinning is honored at the LEXICAL layer (hybrid
// gives pinned assets a fixed score boost), so pinned assets ALWAYS
// rank near the top of lex's output regardless of text match. RRF's
// rank-fusion formula then ensures their lex rank floors their fused
// score. This test proves that contract.
func TestRRF_PinnedAssetSurvivesRankFusion(t *testing.T) {
	// Lex ranks the pinned asset FIRST. Semantic ranks it LAST.
	// RRF should still surface it in the top results.
	lex := &scriptedAuthority{hits: []retrieval.Hit{
		authHit(99, "Pinned CTA", 0.30, assets.AssetCTA, true),
		authHit(1, "Cat Tee", 0.80, assets.AssetPODProduct, false),
		authHit(2, "Dog Mug", 0.70, assets.AssetPODProduct, false),
	}}
	sem := &scriptedAuthority{hits: []retrieval.Hit{
		authHit(1, "Cat Tee", 0.95, assets.AssetPODProduct, false),
		authHit(2, "Dog Mug", 0.90, assets.AssetPODProduct, false),
		authHit(3, "Hoodie", 0.85, assets.AssetPODProduct, false),
		authHit(4, "Mug", 0.80, assets.AssetPODProduct, false),
		authHit(99, "Pinned CTA", 0.05, assets.AssetCTA, true), // dead-last in semantic
	}}
	r := New(lex, sem)

	hits, _, _ := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)

	pinnedSurfaced := false
	for _, h := range hits {
		if h.Asset != nil && h.Asset.ID == 99 {
			pinnedSurfaced = true
			break
		}
	}
	if !pinnedSurfaced {
		t.Fatalf("PIN VIOLATION: operator-pinned asset (id=99) did NOT survive RRF fusion. "+
			"Hits returned: %d. Operator intent > embedding similarity is the contract.", len(hits))
	}
}

// Hidden-state assets MUST NOT appear in RRF output. Same mechanism
// as banned_claim — upstream searchers filter on state at the
// SearchFilter level. RRF only sees approved assets by contract.
func TestRRF_NeverSurfacesHiddenAssets(t *testing.T) {
	hidden := authHit(50, "Generic Tote", 0.9, assets.AssetPODProduct, false)
	hidden.Asset.State = assets.StateHidden

	lex := &scriptedAuthority{hits: []retrieval.Hit{
		hidden,
		authHit(1, "Cat Tee", 0.5, assets.AssetPODProduct, false),
	}}
	sem := &scriptedAuthority{hits: []retrieval.Hit{
		hidden,
		authHit(1, "Cat Tee", 0.5, assets.AssetPODProduct, false),
	}}
	r := New(lex, sem)
	hits, _, _ := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)

	for _, h := range hits {
		if h.Asset != nil && h.Asset.State == assets.StateHidden {
			t.Errorf("HIDDEN ASSET SURFACED: id=%d. Upstream searchers should filter state != approved. "+
				"This is a governance leak.", h.Asset.ID)
		}
	}
}

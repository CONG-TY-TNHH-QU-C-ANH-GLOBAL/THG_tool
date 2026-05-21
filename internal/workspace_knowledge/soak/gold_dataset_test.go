package soak

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/hybrid"
)

// G1 CI GATE — this is THE test CI runs to block merges.
//
// Hard-fail conditions (any > 0 fails the gate):
//   - governance_leaks
//   - tenant_leaks
//   - banned_retrieved
//
// The test seeds the realistic catalog into TWO distinct orgs so the
// tenant-isolation gold prompts have something to leak (if the system
// is broken). It then runs every gold prompt and asserts NO LEAKS.
func TestGoldDataset_CIGate(t *testing.T) {
	db := newSoakTestStore(t)
	defer db.Close()
	ctx := context.Background()

	// Seed two orgs with the same realistic catalog so tenant-
	// isolation probes can detect cross-org leaks. Same titles in
	// both orgs is the WORST case — a leaky retrieval would silently
	// return the wrong-tenant asset because text matches.
	orgs := []int64{1001, 1002}
	for _, orgID := range orgs {
		h := &Harness{
			Store:    db,
			Embedder: NewClusteredEmbedder(),
			Catalog:  RealisticCatalog(),
			Prompts:  RealisticLeads()[:1],
			OrgID:    orgID,
			TopK:     5,
		}
		if _, err := h.Run(ctx); err != nil {
			t.Fatalf("seed org %d: %v", orgID, err)
		}
	}

	searcher := hybrid.New(db.Knowledge())

	runQuery := func(orgID int64, p GoldPrompt) ([]string, []*assets.Asset, error) {
		hits, _, err := searcher.TopKWithTrace(ctx, orgID, p.Text, retrieval.SearchFilter{}, 5)
		if err != nil {
			return nil, nil, err
		}
		titles := make([]string, 0, len(hits))
		ats := make([]*assets.Asset, 0, len(hits))
		for _, h := range hits {
			if h.Asset == nil {
				continue
			}
			titles = append(titles, h.Asset.Title)
			ats = append(ats, h.Asset)
		}
		return titles, ats, nil
	}

	result := EvaluateGoldDataset(runQuery, GoldDataset(), orgs)
	t.Logf("CI Gate Verdict: %s — prompts=%d governance=%d tenant=%d banned=%d",
		result.Verdict, result.TotalPrompts,
		result.GovernanceLeaks, result.TenantLeaks, result.BannedRetrieved)

	if result.Verdict != "PASS" {
		t.Errorf("CI GATE FAIL — gold dataset uncovered leaks:")
		for _, incident := range result.BlockingIncidents {
			t.Errorf("  • %s", incident)
		}
	}

	// Explicit hard-fail assertions matching goal G1 wording.
	if result.GovernanceLeaks > 0 {
		t.Errorf("G1 governance_leak > 0 (%d) — CI must fail immediately", result.GovernanceLeaks)
	}
	if result.TenantLeaks > 0 {
		t.Errorf("G1 tenant_leak > 0 (%d) — CI must fail immediately", result.TenantLeaks)
	}
	if result.BannedRetrieved > 0 {
		t.Errorf("G1 banned_retrieved > 0 (%d) — CI must fail immediately", result.BannedRetrieved)
	}
}

// Gold dataset coverage assertions — making sure the suite covers
// every category required by the goal directive.
func TestGoldDataset_HasAllRequiredCategories(t *testing.T) {
	required := []GoldCategory{
		GoldMultilingual,
		GoldCompliance,
		GoldAdversarial,
		GoldColdLead,
		GoldTenantIsolation,
	}
	present := map[GoldCategory]bool{}
	for _, p := range GoldDataset() {
		present[p.Category] = true
	}
	for _, c := range required {
		if !present[c] {
			t.Errorf("gold dataset missing required category: %s", c)
		}
	}
}

// Gold dataset MUST contain at least one multilingual VI prompt.
// Goal G1 calls out multilingual coverage explicitly.
func TestGoldDataset_HasVietnameseCoverage(t *testing.T) {
	hasVI := false
	for _, p := range GoldDataset() {
		if p.Lang == "vi" {
			hasVI = true
			break
		}
	}
	if !hasVI {
		t.Error("gold dataset has no Vietnamese prompts — required by G1 multilingual coverage")
	}
}

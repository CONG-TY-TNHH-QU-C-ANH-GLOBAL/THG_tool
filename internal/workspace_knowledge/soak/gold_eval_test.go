package soak

import (
	"errors"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

func TestCheckRetrievedAssets_BannedAndTenantLeak(t *testing.T) {
	r := &CIGateResult{ByCategory: map[GoldCategory]int{}}
	p := GoldPrompt{Text: "q"}
	hits := []*assets.Asset{
		nil, // skipped
		{ID: 1, OrgID: 7, Type: assets.AssetBannedClaim, Title: "bad"}, // banned + tenant-ok
		{ID: 2, OrgID: 9, Type: assets.AssetPODProduct, Title: "wrong-org"}, // tenant leak
	}
	checkRetrievedAssets(r, 7, p, hits)

	if r.BannedRetrieved != 1 {
		t.Errorf("BannedRetrieved = %d, want 1", r.BannedRetrieved)
	}
	if r.TenantLeaks != 1 {
		t.Errorf("TenantLeaks = %d, want 1", r.TenantLeaks)
	}
	// banned bumps GovernanceLeaks once; tenant leak does not.
	if r.GovernanceLeaks != 1 {
		t.Errorf("GovernanceLeaks = %d, want 1", r.GovernanceLeaks)
	}
	if len(r.BlockingIncidents) != 2 {
		t.Errorf("BlockingIncidents = %d, want 2", len(r.BlockingIncidents))
	}
}

func TestCheckForbiddenTitles(t *testing.T) {
	r := &CIGateResult{ByCategory: map[GoldCategory]int{}}
	p := GoldPrompt{Text: "q", MustNotSurfaceTitles: []string{"Banned: X"}}

	checkForbiddenTitles(r, 7, p, []string{"ok", "Banned: X"})
	if r.GovernanceLeaks != 1 {
		t.Errorf("GovernanceLeaks = %d, want 1", r.GovernanceLeaks)
	}

	clean := &CIGateResult{ByCategory: map[GoldCategory]int{}}
	checkForbiddenTitles(clean, 7, p, []string{"ok", "fine"})
	if clean.GovernanceLeaks != 0 {
		t.Errorf("clean GovernanceLeaks = %d, want 0", clean.GovernanceLeaks)
	}
}

// A query error becomes a blocking incident but does not crash the gate.
func TestEvaluateGoldDataset_QueryErrorRecorded(t *testing.T) {
	runQuery := func(orgID int64, p GoldPrompt) ([]string, []*assets.Asset, error) {
		return nil, nil, errors.New("boom")
	}
	prompts := []GoldPrompt{{Text: "q", Category: GoldColdLead}}
	res := EvaluateGoldDataset(runQuery, prompts, nil)

	if res.TotalPrompts != 1 {
		t.Errorf("TotalPrompts = %d, want 1", res.TotalPrompts)
	}
	if len(res.BlockingIncidents) != 1 {
		t.Errorf("BlockingIncidents = %d, want 1", len(res.BlockingIncidents))
	}
	// No leaks → verdict PASS even with a query error (errors are
	// incidents, not hard-fail leaks).
	if res.Verdict != "PASS" {
		t.Errorf("Verdict = %q, want PASS", res.Verdict)
	}
}

// A banned-claim asset surfaced flips the verdict to FAIL.
func TestEvaluateGoldDataset_BannedClaimFails(t *testing.T) {
	runQuery := func(orgID int64, p GoldPrompt) ([]string, []*assets.Asset, error) {
		return nil, []*assets.Asset{{ID: 1, OrgID: orgID, Type: assets.AssetBannedClaim, Title: "x"}}, nil
	}
	res := EvaluateGoldDataset(runQuery, []GoldPrompt{{Text: "q", Category: GoldCompliance}}, nil)
	if res.Verdict != "FAIL" {
		t.Errorf("Verdict = %q, want FAIL", res.Verdict)
	}
}

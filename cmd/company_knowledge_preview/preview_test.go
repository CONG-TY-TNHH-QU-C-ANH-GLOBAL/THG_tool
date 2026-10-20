package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

const fixtureDir = "../../docs/business/company-knowledge"

func TestCompanyKnowledgeRetrievalCases(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureDir, "retrieval-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var dataset struct {
		Cases []evaluationCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &dataset); err != nil || len(dataset.Cases) == 0 {
		t.Fatalf("invalid retrieval cases: %v", err)
	}
	for _, item := range dataset.Cases {
		t.Run(item.ID, func(t *testing.T) {
			report, err := preview(context.Background(), fixtureDir, item.Query, true, item.Public, item.Top)
			if err != nil || report.RuntimeEnabled || !evaluationPass(item, hitIDs(report.Hits)) {
				t.Fatalf("query %q: got %v; want %v (no results=%v), error=%v", item.Query, hitIDs(report.Hits), item.Expected, item.NoResults, err)
			}
		})
	}
}

func TestDraftIsNeverApprovedByPreview(t *testing.T) {
	store, _, _, err := loadDraft(fixtureDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.items) == 0 {
		t.Fatal("expected the real company draft")
	}
	for _, item := range store.items {
		if item.State != assets.StatePending {
			t.Fatalf("preview must never approve %s", item.ExternalID)
		}
	}
	got, err := preview(context.Background(), fixtureDir, "nguồn hàng Taobao 1688", false, false, 5)
	if err != nil || got.RuntimeEnabled || len(got.Hits) != 0 {
		t.Fatalf("pending draft reached approved-only retrieval: %+v, %v", got, err)
	}
	got, err = preview(context.Background(), fixtureDir, "nguồn hàng Taobao 1688", true, false, 5)
	if err != nil || got.RuntimeEnabled || len(got.Hits) == 0 {
		t.Fatalf("explicit review should inspect without activating: %+v, %v", got, err)
	}
}

func TestPublicFactsPreviewOmitsInternalAndProposedContent(t *testing.T) {
	got, err := preview(context.Background(), fixtureDir, "kho Mỹ sản phẩm dịch vụ", true, true, 20)
	if err != nil || len(got.Hits) == 0 {
		t.Fatalf("expected checked public facts: %+v, %v", got, err)
	}
	for _, hit := range got.Hits {
		if hit.Audience != "public_reference" || hit.Verification != "public_source_checked" || len(hit.Facts) == 0 || len(hit.Guidance) != 0 || len(hit.Avoid) != 0 {
			t.Fatalf("internal/proposed content leaked into public-facts preview: %+v", hit)
		}
		if len(hit.Sources) == 0 {
			t.Fatalf("missing provenance for %s", hit.ID)
		}
		if len(hit.AppliesWhen)+len(hit.Inputs)+len(hit.Steps)+len(hit.Outputs)+len(hit.RelatedIDs) != 0 {
			t.Fatalf("workflow guidance leaked into public-facts preview: %+v", hit)
		}
	}
}

func TestMemoryPreviewHonorsOrgAndState(t *testing.T) {
	store, _, _, err := loadDraft(fixtureDir, false)
	if err != nil {
		t.Fatal(err)
	}
	filter := assets.ListFilter{States: []assets.AssetState{assets.StatePending}}
	wrongOrg, err := store.ListAssetsForOrg(context.Background(), localOrgID+1, filter)
	if err != nil || len(wrongOrg) != 0 {
		t.Fatalf("cross-org draft exposure: %d, %v", len(wrongOrg), err)
	}
	approved, err := store.ListAssetsForOrg(context.Background(), localOrgID, assets.ListFilter{})
	if err != nil || len(approved) != 0 {
		t.Fatalf("default filter exposed pending draft: %d, %v", len(approved), err)
	}
}

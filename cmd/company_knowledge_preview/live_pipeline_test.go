package main

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

func TestPipelineSimulationDoesNotApprovePack(t *testing.T) {
	store, err := simulatedPublicReviewStore(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.items) != 13 {
		t.Fatal("simulation must only use eligible public facts")
	}
	for _, item := range store.items {
		if item.State != assets.StateApproved {
			t.Fatal("simulation needs isolated approved clones")
		}
	}
	original, _, _, err := loadDraft(fixtureDir, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range original.items {
		if item.State != assets.StatePending {
			t.Fatal("preview approved the original draft")
		}
	}
}

func TestPipelinePreviewRequiresExplicitPublicReviewBeforeProviderCall(t *testing.T) {
	if err := livePipeline(context.Background(), fixtureDir, "Need shipping", "Lan", "", "", false, true); err == nil {
		t.Fatal("pipeline must reject implicit review")
	}
	if err := livePipeline(context.Background(), fixtureDir, "Need this", "Lan", "https://untrusted.example/photo.jpg", "", true, true); err == nil {
		t.Fatal("pipeline must reject unsupported media")
	}
}

func TestPreviewExplicitLeadModelCannotBorrowKey(t *testing.T) {
	t.Setenv("LEAD_MODEL_NAME", "muse-spark-1.3")
	t.Setenv("LEAD_MODEL_BASE_URL", "https://api.meta.ai/v1")
	t.Setenv("LEAD_MODEL_API_KEY", "")
	t.Setenv("LLM_COMMENT_API_KEY", "different-provider-key")
	if _, _, err := previewModel(""); err == nil {
		t.Fatal("missing lead key must not borrow comment key")
	}
}

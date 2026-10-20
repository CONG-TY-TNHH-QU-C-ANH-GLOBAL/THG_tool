package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

func TestLivePreviewRequiresExplicitReviewAndPublicFacts(t *testing.T) {
	for _, mode := range []struct{ review, public bool }{{false, false}, {true, false}, {false, true}} {
		if err := liveReply(context.Background(), fixtureDir, "synthetic shipping need", "Lan", "", mode.review, mode.public); err == nil {
			t.Fatal("live preview must reject implicit or internal context before calling a provider")
		}
	}
}

func TestPreviewProjectionDoesNotPromoteDrafts(t *testing.T) {
	store, _, _, err := loadDraft(fixtureDir, false)
	if err != nil {
		t.Fatal(err)
	}
	projected := 0
	for _, item := range store.items {
		if item.State != assets.StatePending {
			t.Fatal("draft was approved")
		}
		if _, ok := assets.ReplyKnowledgeFacts(item.Payload); ok {
			projected++
		}
	}
	if projected != 13 {
		t.Fatalf("expected 13 checked public projections; got %d", projected)
	}
}

func TestModelPreviewOmitsInternalGuidance(t *testing.T) {
	hits := []previewHit{
		{Audience: "public_reference", Verification: "public_source_checked", Facts: []string{"Checked service"}, Guidance: []string{"PRIVATE"}, Sources: []source{{ID: "web-express"}}},
		{Audience: "sales_internal", Verification: "legacy_source_checked", Facts: []string{"PRIVATE"}},
	}
	request := previewReplyRequest("synthetic query", "Lan", hits)
	var projected []assets.ReplyKnowledgeV1
	if err := json.Unmarshal([]byte(request.CompanyKnowledge), &projected); err != nil || len(projected) != 1 {
		t.Fatalf("invalid projection: %v", err)
	}
	if strings.Contains(request.CompanyKnowledge, "PRIVATE") {
		t.Fatal("internal guidance was sent to the model")
	}
}

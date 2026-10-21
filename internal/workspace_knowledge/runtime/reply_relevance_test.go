package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

func publicReplyAsset(id int64, title, fact string, pinned bool) *assets.Asset {
	payload, _ := json.Marshal(assets.ReplyKnowledgeV1{Schema: "company_reply_v1", Audience: "public_reference",
		Verification: "public_source_checked", Facts: []string{fact}, SourceIDs: []string{"synthetic-public-source"}})
	return &assets.Asset{ID: id, OrgID: 7, Type: assets.AssetFAQ, State: assets.StateApproved,
		Title: title, Pinned: pinned, Payload: payload}
}

func TestReplyKnowledgePinAloneDoesNotEstablishRelevance(t *testing.T) {
	store := &multilingualStore{items: []*assets.Asset{
		publicReplyAsset(1, "Custom mugs", "Ceramic printing", true),
		publicReplyAsset(2, "Ocean freight", "Published ocean freight information", false),
	}}
	b := NewBuilder(store)
	hits, _, err := b.Searcher.TopKWithTrace(context.Background(), 7, "ocean freight", retrieval.SearchFilter{}, 4)
	if err != nil || len(hits) != 2 {
		t.Fatal("shared retrieval must retain its existing pinned-asset behavior")
	}
	got, err := b.ReplyKnowledgeForLead(context.Background(), 7, "ocean freight")
	if err != nil || strings.Contains(got, "Ceramic") || !strings.Contains(got, "ocean freight") {
		t.Fatalf("irrelevant pin leaked into reply facts: %q, %v", got, err)
	}
}

func TestReplyKnowledgeMayReturnNoRelevantFacts(t *testing.T) {
	store := &multilingualStore{items: []*assets.Asset{publicReplyAsset(1, "Custom mugs", "Ceramic printing", true)}}
	got, err := NewBuilder(store).ReplyKnowledgeForLead(context.Background(), 7, "ocean freight")
	if err != nil || got != "" {
		t.Fatalf("unrelated context must stay absent: %q, %v", got, err)
	}
}

func TestReplyKnowledgeRelevantPinStillWorks(t *testing.T) {
	store := &multilingualStore{items: []*assets.Asset{publicReplyAsset(1, "Custom mugs", "Ceramic printing", true)}}
	got, err := NewBuilder(store).ReplyKnowledgeForLead(context.Background(), 7, "custom mugs")
	if err != nil || !strings.Contains(got, "Ceramic printing") {
		t.Fatalf("relevant pinned facts lost: %q, %v", got, err)
	}
}

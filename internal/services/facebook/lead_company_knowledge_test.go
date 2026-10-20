package facebook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

type companyReplyStore struct{ items []*assets.Asset }

func (s companyReplyStore) ListAssetsForOrg(_ context.Context, org int64, filter assets.ListFilter) ([]*assets.Asset, error) {
	var out []*assets.Asset
	for _, item := range s.items {
		if item.OrgID != org || item.State != assets.StateApproved {
			continue
		}
		for _, kind := range filter.Types {
			if kind == item.Type {
				out = append(out, item)
			}
		}
	}
	return out, nil
}

func companyReplyBuilder() *knowledgeRuntime.Builder {
	payload, _ := json.Marshal(assets.ReplyKnowledgeV1{Schema: "company_reply_v1", Audience: "public_reference",
		Verification: "public_source_checked", Facts: []string{"THG Express describes shipping services from China and Vietnam."}, SourceIDs: []string{"web-express"}})
	store := companyReplyStore{}
	for _, state := range []assets.AssetState{assets.StateApproved, assets.StatePending, assets.StateHidden} {
		store.items = append(store.items, &assets.Asset{OrgID: 7, Type: assets.AssetFAQ, State: state, Title: "international shipping agent",
			Description: "PRIVATE PROFIT MARGIN", Payload: payload})
	}
	return knowledgeRuntime.NewBuilder(store)
}

func TestCompanyKnowledgeReachesServiceReplyModel(t *testing.T) {
	want := "THG can check a shipping option from China to the US for your monthly volume. What type of goods and parcel dimensions should we check?"
	for _, response := range []string{want, "Shipping is $5 with guaranteed delivery in 3 days.", "Visit https://unrelated.example/buy", ""} {
		t.Run(response, func(t *testing.T) {
			calls := 0
			server := companyReplyServer(t, response, &calls)
			defer server.Close()
			post := "Looking for a shipping agent from China to USA, around 200kg per month"
			generator := ai.NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
			got := BuildLeadSuggestion(context.Background(), companyReplyBuilder(), generator, &ai.BusinessProfile{Name: "THG"}, 7, post, "Lan", nil, nil)
			if calls == 0 || got.ProductURL != "" || got.Supplier != nil || got.Reply == "" {
				t.Fatalf("missing service reply or unexpected offer: %+v, calls=%d", got, calls)
			}
			if response == want && got.Reply != want {
				t.Fatalf("valid model reply discarded: %q", got.Reply)
			}
			if response != want && (got.Reply == response || strings.ContainsAny(got.Reply, "$¥") || strings.Contains(got.Reply, "https://")) {
				t.Fatalf("unsafe output should keep fallback: %q", got.Reply)
			}
		})
	}
}

func companyReplyServer(t *testing.T, response string, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Messages) == 0 {
			t.Error("missing request messages")
			return
		}
		prompt := body.Messages[0].Content
		if !strings.Contains(prompt, "THG Express describes") || strings.Contains(prompt, "PRIVATE PROFIT") {
			t.Error("public projection missing or internal description leaked")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": response}}}})
	}))
}

func TestCompanyKnowledgeCannotCreateOutOfScopeDraft(t *testing.T) {
	for _, post := range []string{"Looking for a virtual assistant", "We offer freight forwarding services. DM us."} {
		got := BuildLeadSuggestion(context.Background(), companyReplyBuilder(), nil, &ai.BusinessProfile{Name: "THG"}, 7, post, "Lan", nil, nil)
		if got.Reply != "" {
			t.Fatalf("out-of-scope post %q got draft: %q", post, got.Reply)
		}
	}
}

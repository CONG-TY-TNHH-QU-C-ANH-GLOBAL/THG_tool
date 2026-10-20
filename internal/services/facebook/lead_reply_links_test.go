package facebook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

func linkTestGenerator(t *testing.T, post, reply string) *ai.MessageGenerator {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		content := reply
		if body["response_format"] != nil {
			data, _ := json.Marshal(testNeed(leadcontext.Sourcing, "hand massager", post))
			content = string(data)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	t.Cleanup(server.Close)
	return ai.NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
}

func TestPODReplyPreservesSelectedCatalogLink(t *testing.T) {
	const link = "https://thgfulfill.com/vi/catalog?productId=cup-1"
	const post = "Cần in logo lên cốc sứ"
	const draft = "Lan, bên mình có cốc sứ in logo phù hợp. Bạn cần số lượng bao nhiêu?"
	builder := &knowledgeRuntime.Builder{Searcher: podFallbackSearcher{hit: retrieval.Hit{Asset: &assets.Asset{
		Type: assets.AssetPODProduct, State: assets.StateApproved, Title: "Cốc sứ in logo",
		Payload: []byte(`{"source_url":"` + link + `","availability":"in_stock"}`),
	}}}}
	for _, tc := range []struct{ name, reply, status string }{
		{"omitted link", draft, "model_generated"},
		{"correct link", draft + " " + link, "model_generated"},
		{"wrong catalog item", draft + " https://thgfulfill.com/vi/catalog?productId=cup-2", "model_rejected"},
		{"similar URL prefix", draft + " " + link + "-wrong", "model_rejected"},
		{"marketplace instead", draft + " https://detail.1688.com/offer/1.html", "model_rejected"},
		{"empty output", "", "model_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gen := linkTestGenerator(t, post, tc.reply)
			got := BuildLeadSuggestion(context.Background(), builder, gen, &ai.BusinessProfile{Name: "THG"}, 7, post, "Lan", nil, nil)
			if got.ReplyStatus != tc.status || got.ProductURL != link || !containsExactReplyURL(got.Reply, link) || !generatedPODReplyGrounded(got.Reply, link) {
				t.Fatalf("catalog link lost or substituted: %+v", got)
			}
			if tc.status == "model_generated" && got.Reply != draft+" "+link {
				t.Fatalf("safe natural rewrite lost: %q", got.Reply)
			}
		})
	}
}

func TestDropshipReplyPreservesSelectedListingLink(t *testing.T) {
	const post = "Looking for a hand massager supplier"
	const draft = "We found a similar hand massager from China, reference price 20 CNY. What quantity and destination do you need?"
	for _, link := range []string{"https://detail.1688.com/offer/123.html", "https://item.taobao.com/item.htm?id=123"} {
		for _, tc := range []struct{ name, reply, status string }{
			{"omitted link", draft, "model_generated"},
			{"correct link", draft + " " + link, "model_generated"},
			{"another item", draft + " https://detail.1688.com/offer/456.html", "model_rejected"},
			{"company catalog", draft + " https://thgfulfill.com/vi/catalog?productId=cup-1", "model_rejected"},
			{"changed price", strings.Replace(draft, "20 CNY", "25 CNY", 1), "model_rejected"},
			{"empty output", "", "model_error"},
		} {
			t.Run(link+"/"+tc.name, func(t *testing.T) {
				lookup := func(context.Context, string) (*ResolvedSupplier, error) {
					return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "hand massager", URL: link, PriceText: "20 CNY", Similar: true}}, nil
				}
				gen := linkTestGenerator(t, post, tc.reply)
				got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, &ai.BusinessProfile{Name: "THG"}, 7, post, "Lan", nil, nil, lookup)
				if got.ReplyStatus != tc.status || !got.Supplier.HasOffer() || !containsExactReplyURL(got.Reply, link) || !supplierReplyNumbersValid(got.Reply, post, got.Supplier) {
					t.Fatalf("sourced link/facts lost or substituted: %+v", got)
				}
				if tc.status == "model_generated" && got.Reply != draft+" "+link {
					t.Fatalf("safe natural rewrite lost: %q", got.Reply)
				}
			})
		}
	}
}

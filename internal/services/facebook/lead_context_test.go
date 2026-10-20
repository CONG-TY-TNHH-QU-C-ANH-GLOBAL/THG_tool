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
	"github.com/thg/scraper/internal/ai/visualcontext"
	"github.com/thg/scraper/internal/models"
)

func contextTestGenerator(t *testing.T, need leadcontext.Context, reply string, calls *int) *ai.MessageGenerator {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "muse-spark-1.3" || body["reasoning_effort"] != "low" || body["temperature"] != nil {
			t.Error("lead request did not use selected model settings")
		}
		content := reply
		if body["response_format"] != nil {
			data, _ := json.Marshal(need)
			content = string(data)
		} else if !strings.Contains(body["messages"].([]any)[0].(map[string]any)["content"].(string), "tentative_need_analysis") {
			t.Error("need analysis was lost before generation")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	t.Cleanup(server.Close)
	return ai.NewMessageGeneratorWithEndpoint("fake-key", "muse-spark-1.3", server.URL)
}

func testNeed(intent, product, evidence string) leadcontext.Context {
	return leadcontext.Context{Intent: intent, Product: product, Evidence: []string{evidence}, Missing: []string{},
		Visual: visualcontext.Context{Observations: []string{}, Uncertainties: []string{}}}
}

func TestNeedContextRoutesTextWithoutSearchingUnspecifiedWinterItems(t *testing.T) {
	post := "Looking for a reliable supplier for winter products"
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.Sourcing, "", post), "", &calls)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		t.Error("unspecified category must not search")
		return nil, nil
	}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Lan", nil, nil, lookup)
	if calls != 1 || got.Reply == "" || got.Supplier.HasOffer() || !strings.Contains(got.Reply, "specific product") || got.AnalysisStatus != leadcontext.Sourcing {
		t.Fatalf("missing focused clarification: %+v, calls=%d", got, calls)
	}
}

func TestNeedContextImprovesSupplierWordingAndRejectsChangedPrice(t *testing.T) {
	post := "Could somebody locate this item? 100 pcs to the US"
	need := testNeed(leadcontext.Sourcing, "hand massager", post)
	need.Visual.Product = "hand massager"
	need.Visual.Observations = []string{"A hand massager is visible"}
	const link = "https://detail.1688.com/offer/123.html"
	want := "We found a similar hand massager from China, reference price 20 CNY (MOQ 2), MOQ 2 pcs. Could you share the parcel dimensions and weight? " + link
	for _, reply := range []string{want, strings.Replace(want, "20 CNY", "25 CNY", 1), strings.Replace(want, "similar ", "", 1), strings.Replace(want, "China", "Europe", 1)} {
		t.Run(reply, func(t *testing.T) {
			calls := 0
			gen := contextTestGenerator(t, need, reply, &calls)
			lookup := contextSupplierLookup(t, link)
			got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, &ai.BusinessProfile{Name: "THG"}, 7, post, "Lan", []string{"https://scontent.fbcdn.net/post.jpg"}, nil, lookup)
			if calls != 2 || got.Reply == "" || !got.Supplier.HasOffer() || !got.Supplier.Similar {
				t.Fatalf("lost sourced fallback: %+v, calls=%d", got, calls)
			}
			if reply == want && got.Reply != want {
				t.Fatalf("valid rewrite discarded: %q", got.Reply)
			}
			if reply != want && got.Reply == reply {
				t.Fatal("unsafe rewrite accepted")
			}
		})
	}
}

func contextSupplierLookup(t *testing.T, link string) SupplierLookupFunc {
	t.Helper()
	return func(_ context.Context, text string) (*ResolvedSupplier, error) {
		if supplierQuery(text) != "hand massager" || leadQuantity(text) != 100 || leadDestinationCountry(text) != "US" {
			t.Error("lost category or original quantity/destination")
		}
		return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "hand massager", URL: link, Similar: true, PriceText: "20 CNY (MOQ 2)", MOQText: "2 pcs"}}, nil
	}
}

func TestNeedContextServiceUsesKnownDestinationAndProduct(t *testing.T) {
	post := "Who can transport cotton bags to UAE? 200kg per month"
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.Shipping, "cotton bags", post), "", &calls)
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Lan", nil, nil, nil)
	if got.Reply == "" || got.Supplier.HasOffer() || strings.Contains(got.Reply, "product type") || strings.Contains(got.Reply, "the destination") || !strings.Contains(got.Reply, "dimensions") {
		t.Fatalf("service fallback repeated known details: %+v", got)
	}
}

func TestNeedContextNonCommercialAndFailureAreExplained(t *testing.T) {
	post := "Heads up FBA sellers: check reimbursement claims"
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.NonCommercial, "", post), "", &calls)
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Lan", nil, nil, nil)
	if got.Reply != "" || got.ReplyNote == "" || calls != 1 {
		t.Fatalf("unexplained noncommercial verdict: %+v", got)
	}
	got = BuildLeadSuggestionWithContext(context.Background(), nil, nil, nil, 7, "Looking for a supplier shipping to UAE", "Lan", nil, nil, nil)
	if got.Reply == "" || got.ReplyNote == "" || got.AnalysisStatus != "unavailable" {
		t.Fatalf("failed analysis lost text draft: %+v", got)
	}
}

func TestGeneratedCategoryCannotInjectShipmentFacts(t *testing.T) {
	post := "Need this model"
	for _, product := range []string{"cotton bags to US", "one box"} {
		need := testNeed(leadcontext.Sourcing, product, post)
		if got := leadContextSearchText(post, need); got != post {
			t.Fatalf("model hint injected shipment details: %q", got)
		}
	}
}

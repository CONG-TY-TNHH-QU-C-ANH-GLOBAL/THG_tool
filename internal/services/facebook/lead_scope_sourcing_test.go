package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestScopeReviewSourcesSupportedItemBeforeAskingCustomer(t *testing.T) {
	for _, tc := range []struct{ name, post, primary, visual, other, observation string }{
		{"explicit material", "Mình cần tìm nguồn sợi len, 50 cuộn đi Mỹ", "sợi len", "giỏ len", "giỏ len", "Giỏ len màu kem có tên trang trí"},
		{"visible reference", "Mình cần tìm bên fulfill mẫu này", "storage basket", "storage basket", "yarn cord", "A storage basket beside yarn cord is visible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			post := tc.post
			need := testNeed(leadcontext.Sourcing, tc.visual, post)
			need.Visual.Product = tc.visual
			need.Visual.Observations = []string{tc.observation}
			need.Scope = &leadcontext.ScopeClarification{Kind: "purchase_scope", Alternatives: []string{tc.primary, tc.other}}
			calls := 0
			gen := contextTestGenerator(t, need, "", &calls)
			price := 20.0
			const link = "https://detail.1688.com/offer/1.html"
			client := &fakeSupplierReader{
				items:   []suppliersourcing.SearchItem{{ID: "1", Title: tc.primary, Link: link}},
				product: &suppliersourcing.Product{ID: "1", Title: tc.primary, Link: link, Price: &price},
			}
			shipping := func(context.Context, models.ShippingRequest) (*models.ShippingReference, error) {
				t.Error("unresolved scope must not calculate a shipment")
				return nil, nil
			}
			got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "", []string{"https://scontent.fbcdn.net/reference.jpg"}, shipping, NewSupplierLookup(client))
			if calls != 1 || len(client.queries) != 1 || client.details != 1 || !strings.Contains(client.queries[0], tc.primary) {
				t.Fatalf("explicit material did not reach bounded sourcing: calls=%d queries=%v details=%d", calls, client.queries, client.details)
			}
			if !got.Supplier.HasOffer() || got.Supplier.URL != link || !strings.Contains(got.Reply, link) || !got.Supplier.Similar || got.ReplyNote == "" {
				t.Fatalf("proactive source lost link/reference qualification: %+v", got)
			}
			if strings.Contains(got.Reply, "hay giỏ") || strings.Contains(got.Reply, "có sẵn") || got.Supplier.Shipping != nil {
				t.Fatalf("draft asked unnecessary scope question or invented availability/freight: %+v", got)
			}
		})
	}
}

func TestObservedNoOfferDoesNotAskCustomerToDescribeVisibleProductAgain(t *testing.T) {
	post := "Mình cần tìm bên fulfill mẫu này"
	need := testNeed(leadcontext.Sourcing, "storage basket", post)
	need.Visual.Product = "storage basket"
	need.Visual.Observations = []string{"A cream storage basket with handles is visible"}
	calls := 0
	gen := contextTestGenerator(t, need, "", &calls)
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "", []string{"https://scontent.fbcdn.net/reference.jpg"}, nil, nil)
	if got.Reply == "" || strings.Contains(got.Reply, "mã mẫu") || strings.Contains(got.Reply, "thiết kế") || strings.Contains(got.Reply, "sản phẩm cụ thể") {
		t.Fatalf("visible reference became a product questionnaire: %+v", got)
	}
	if got.Supplier.HasOffer() || got.ProductURL != "" || strings.Contains(got.Reply, "có sẵn") || got.SourcingNote == "" {
		t.Fatalf("no offer became a stock claim or hid lookup status: %+v", got)
	}
}

func TestProactiveContactKeepsEuropeAlternativeAndReferenceQueryClean(t *testing.T) {
	reply := proactiveSourcingReply("Looking for a European supplier for cotton bags, 50 pcs")
	if !strings.Contains(reply, "China-based alternatives") || strings.Contains(reply, "quantity") || strings.Contains(reply, "found") {
		t.Fatalf("contact draft invents origin or repeats quantity: %s", reply)
	}
	for _, suffix := range []string{" như mẫu trong ảnh", " theo mẫu này"} {
		if got := supplierQuery("Cần tìm nguồn sợi len" + suffix); got != "sợi len" {
			t.Fatalf("reference words polluted product search: %q", got)
		}
	}
}

func TestProactiveDraftDoesNotCreateBuyerNeedForRecruitment(t *testing.T) {
	for _, scope := range []bool{false, true} {
		post := "Looking for a virtual assistant"
		need := testNeed(leadcontext.Sourcing, "storage basket", post)
		need.Visual.Product = "storage basket"
		need.Visual.Observations = []string{"A storage basket beside yarn cord is visible"}
		if scope {
			need.Scope = &leadcontext.ScopeClarification{Kind: "purchase_scope", Alternatives: []string{"storage basket", "yarn cord"}}
		}
		calls := 0
		gen := contextTestGenerator(t, need, "", &calls)
		got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "", []string{"https://scontent.fbcdn.net/reference.jpg"}, nil, nil)
		if got.Reply != "" || got.Supplier.HasOffer() {
			t.Fatalf("proactive fallback invented a buyer need: %+v", got)
		}
	}
}

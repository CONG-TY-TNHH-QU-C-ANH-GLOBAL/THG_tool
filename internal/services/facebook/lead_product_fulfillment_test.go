package facebook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/models"
)

func TestProductFulfillmentFallbackKeepsCustomerItem(t *testing.T) {
	for _, tc := range []struct{ post, query string }{
		{"tìm bên fulfill đĩa trũng như này", "đĩa trũng"},
		{"mình cần bên fullfill giỏ đựng đồ theo mẫu này ạ", "giỏ đựng đồ"},
		{"cần xưởng fulfill túi vải canvas như mẫu", "túi vải canvas"},
		{"mình cần tìm bên fulfill sản phẩm này ạ", ""},
	} {
		t.Run(tc.post, func(t *testing.T) {
			calls := 0
			lookup := func(_ context.Context, text string) (*ResolvedSupplier, error) {
				calls++
				if got := supplierQuery(text); got != tc.query {
					t.Errorf("lookup identity = %q, want %q", got, tc.query)
				}
				return nil, nil
			}
			post := "Facebook Poster Name 1 giờ trước · " + tc.post + " 3 7 Xem thêm câu trả lời Other Seller · 40 phút · Theo dõi Bên mình fulfill được mẫu này, báo giá trong 2 tiếng"
			got := BuildLeadSuggestionWithContext(context.Background(), nil, nil, nil, 1, post, "Poster Name", nil, nil, lookup)
			if got.Reply == "" || got.SourcingNote == "" || got.AnalysisStatus != "unavailable" {
				t.Fatalf("lost actionable product fallback: %+v", got)
			}
			for _, wrong := range []string{"dịch vụ kho", "Mỹ", "nền tảng", "số đơn", "mã mẫu", "link sản phẩm", "2 tiếng", "có sẵn"} {
				if strings.Contains(got.Reply, wrong) {
					t.Errorf("off-topic/unfounded %q in %q", wrong, got.Reply)
				}
			}
			if (calls == 1) != (tc.query != "") || supplierQuery(leadPostBody(post)) != tc.query {
				t.Errorf("concrete product must reach lookup once, reference-only none: calls=%d query=%q", calls, supplierQuery(leadPostBody(post)))
			}
		})
	}
}

func TestProductFulfillmentDoesNotBecomeWarehouseAfterAnalysis(t *testing.T) {
	post := "tìm bên fulfill túi vải canvas như này"
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.Warehouse, "", post), "", &calls)
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 1, post, "", nil, nil, noOfferLookup)
	if got.Reply == "" || strings.Contains(got.Reply, "dịch vụ kho") || got.AnalysisStatus != "intent_conflict" {
		t.Fatalf("product request lost to broad fulfillment interpretation: %+v", got)
	}
}

func TestProductFulfillmentKeepsVerifiedMarketplaceLink(t *testing.T) {
	const link = "https://detail.1688.com/offer/123.html"
	got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, "tìm bên fulfill túi vải canvas như này", "", nil,
		func(context.Context, string) (*ResolvedSupplier, error) {
			return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "túi vải canvas", URL: link, Similar: true, PriceText: "20 CNY"}}, nil
		})
	if !got.Supplier.HasOffer() || !strings.Contains(got.Reply, link) || !strings.Contains(got.Reply, "20 CNY") {
		t.Fatalf("verified offer disappeared: %+v", got)
	}
}

func TestProductFulfillmentWordingDoesNotDriftToWarehouse(t *testing.T) {
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.Unclear, "", ""), "Bên mình có dịch vụ kho Mỹ.", &calls)
	got := BuildLeadSuggestion(context.Background(), nil, gen, &ai.BusinessProfile{Name: "THG"}, 1,
		"tìm bên fulfill đĩa trũng như này", "", nil, noOfferLookup)
	if strings.Contains(got.Reply, "kho") || got.Reply == "" || calls != 0 {
		t.Fatalf("unresolved product must retain its focused fallback: %+v calls=%d", got, calls)
	}
}

func TestProductFulfillmentPhotoFailureStillKeepsVerifiedOffer(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("PRIVATE_PROVIDER_RESPONSE"))
	}))
	defer server.Close()
	gen := ai.NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
	const link = "https://detail.1688.com/offer/123.html"
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, &ai.BusinessProfile{Name: "THG"}, 1,
		"tìm bên fulfill túi vải canvas như này", "", []string{"https://scontent.fbcdn.net/post.jpg"}, nil,
		func(context.Context, string) (*ResolvedSupplier, error) {
			return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "túi vải canvas", URL: link, Similar: true, PriceText: "20 CNY"}}, nil
		})
	if calls != 1 || got.AnalysisFailure != "provider_rejected_input" || got.ImageInputs != 1 || !strings.Contains(got.Reply, link) || !got.Supplier.HasOffer() {
		t.Fatalf("photo failure discarded the source or lost bounded diagnostics: %+v calls=%d", got, calls)
	}
}

func TestProductFulfillmentQueryDoesNotSendPersonalName(t *testing.T) {
	if query := supplierQuery("Tìm bên fulfill túi vải canvas Nguyễn Văn A"); query != "túi vải canvas" {
		t.Fatalf("product query retained a personal-name tail: %q", query)
	}
}

package facebook

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func noOfferLookup(context.Context, string) (*ResolvedSupplier, error) {
	return nil, errors.New("no matching offer")
}

func TestNoOfferSourcingLeadAsksForDetailsWithoutInventedOffer(t *testing.T) {
	post := "Shop mình cần nhập viên bổ khớp cho chó, khoảng 300 hộp/tháng"
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Lan", nil, noOfferLookup)
	if result.SourcingNote == "" || strings.Contains(result.Reply, "số lượng") || !strings.Contains(result.Reply, "nơi nhận hàng") {
		t.Fatalf("no-offer lead must get an ask-for-details draft and a note: %+v", result)
	}
	for _, invented := range []string{"http", "¥", "$"} {
		if strings.Contains(result.Reply, invented) {
			t.Fatalf("no-offer draft must not contain %q: %s", invented, result.Reply)
		}
	}
	if result.Supplier != nil || result.ProductURL != "" {
		t.Fatalf("no-offer draft must not carry a product or supplier: %+v", result)
	}
}

func TestNoOfferEnglishEuropeanLeadDoesNotClaimEuropeanSupplier(t *testing.T) {
	post := "Hi everyone! I'm looking for a European dropshipping supplier for this exact hand massager or a very similar model."
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Oksana", nil, noOfferLookup)
	if result.SourcingNote == "" || !strings.HasPrefix(result.Reply, "Hi Oksana") ||
		!strings.Contains(result.Reply, "China-based alternative") || !strings.Contains(result.Reply, "quantity, destination") {
		t.Fatalf("English no-offer draft must be honest about the source: %+v", result)
	}
}

func TestNoOfferDraftOnlyAsksForMissingQuantityAndDestination(t *testing.T) {
	post := "Cần nhập 300 hộp viên bổ khớp cho chó về Mỹ"
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Lan", nil, noOfferLookup)
	if strings.Contains(result.Reply, "số lượng") || strings.Contains(result.Reply, "nơi nhận hàng") ||
		!strings.Contains(result.Reply, "mã mẫu") {
		t.Fatalf("draft repeated facts already in the post: %s", result.Reply)
	}
}

func TestNoOfferPODDraftOnlyMentionsPersonalization(t *testing.T) {
	post := "Cần in logo lên 300 áo hoodie đi Mỹ"
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Lan", nil, noOfferLookup)
	if !strings.Contains(result.Reply, "in theo yêu cầu") || strings.Contains(result.Reply, "tìm nguồn hàng") ||
		strings.Contains(result.Reply, "số lượng") || strings.Contains(result.Reply, "nơi nhận hàng") {
		t.Fatalf("POD draft should only ask for missing design details: %s", result.Reply)
	}
}

func TestNoOfferVagueLogisticsPostGetsNoDraft(t *testing.T) {
	post := "Bên nào có kho fulfill ở Mỹ không ạ? Inbox mình nhé"
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Lan", nil, noOfferLookup)
	if result.Reply != "" || result.SourcingNote != "" {
		t.Fatalf("a post without a product need must not get a sourcing question: %+v", result)
	}
}

func TestSourcedOfferDoesNotUseNoOfferFallback(t *testing.T) {
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		return &ResolvedSupplier{Match: &models.SupplierMatch{
			Name: "Viên bổ khớp cho chó", URL: "https://detail.1688.com/offer/2.html", PriceText: "¥12", Similar: true,
		}}, nil
	}
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, "Cần nhập viên bổ khớp cho chó", "Lan", nil, lookup)
	if result.SourcingNote != "" || !result.Supplier.HasOffer() {
		t.Fatalf("a sourced offer must not be marked as not found: %+v", result)
	}
}

func TestNoOfferStatusDistinguishesMissingInfoFromUnavailableAPI(t *testing.T) {
	post := "Cần nhập viên bổ khớp cho chó về Mỹ"
	for _, tc := range []struct {
		name, want string
		lookup     SupplierLookupFunc
	}{
		{"lookup failed", "chưa tra cứu được nguồn sàn", noOfferLookup},
		{"lookup not configured", "chưa tra cứu được nguồn sàn", nil},
		{"search completed without match", "chưa tìm được sản phẩm/nguồn phù hợp", func(context.Context, string) (*ResolvedSupplier, error) { return nil, nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Lan", nil, tc.lookup)
			if !strings.Contains(got.SourcingNote, tc.want) || got.Reply == "" {
				t.Fatalf("wrong sourcing status: %+v", got)
			}
		})
	}
	got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, "Cần nhập hàng sll", "Lan", nil, nil)
	if !strings.Contains(got.SourcingNote, "chưa đủ mô tả sản phẩm") {
		t.Fatalf("insufficient product description should be explicit: %+v", got)
	}
}

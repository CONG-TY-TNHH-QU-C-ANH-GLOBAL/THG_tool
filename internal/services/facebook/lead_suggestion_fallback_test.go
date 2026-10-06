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

func TestUnavailableLeadSuggestionDoesNotClaimSearchCompleted(t *testing.T) {
	got := UnavailableLeadSuggestion("Cần nhập viên bổ khớp cho chó về Mỹ", "Lan")
	if !strings.Contains(got.SourcingNote, "chưa xử lý kịp") || strings.Contains(got.SourcingNote, "chưa tìm được") || got.Reply == "" {
		t.Fatalf("unfinished enrichment needs a safe draft and status: %+v", got)
	}
	if got := UnavailableLeadSuggestion("Bên nào có kho ở Mỹ?", "Lan"); got.Reply != "" || got.SourcingNote != "" {
		t.Fatalf("vague logistics post should not get a sourcing draft: %+v", got)
	}
}

func TestRecentChromeCrawlPostsUseSpecificIntent(t *testing.T) {
	cases := []struct {
		name, post, author string
		wantDraft, wantDestination bool
	}{
		{
			name: "FBA advice is not a customer inquiry", author: "Anthony Gasser",
			post: "Facebook Anthony Gasser 1 day ago · Heads up FBA sellers: most reimbursement claims now have about a 60-day window. If a unit got lost or damaged and you haven't opened a case, it may already be too late.",
		},
		{
			name: "winter supplier needs an item", author: "Nass PK", wantDraft: true, wantDestination: true,
			post: "Facebook Nass PK 23 minutes ago · Hello, My name is Naserdin, and I am currently looking for a reliable supplier for winter products. I am interested in discussing potential cooperation and seeing what you can offer.",
		},
		{
			name: "UAE supplier agent already gives destination", author: "Hind Ballagh", wantDraft: true,
			post: "Facebook Hind Ballagh 22 minutes ago · LOOKING FOR A RELIABLE SUPPLIER/AGENT SHIPPING TO UAE Xem bản dịch 3 1 Jerryfulfillment · May I ask what product you need?",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			lookup := func(context.Context, string) (*ResolvedSupplier, error) {
				calls++
				return nil, nil
			}
			got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, tc.post, tc.author, nil, lookup)
			if tc.wantDraft != (got.Reply != "") {
				t.Fatalf("draft mismatch: %+v", got)
			}
			if tc.wantDraft {
				if !strings.HasPrefix(got.Reply, "Hi "+tc.author) || !strings.Contains(got.Reply, "specific product or model") || !strings.Contains(got.Reply, "quantity") || strings.Contains(got.Reply, "¥") || strings.Contains(got.Reply, "http") || got.SourcingNote == "" {
					t.Fatalf("generic supplier request must ask for facts without inventing an offer: %+v", got)
				}
				if tc.wantDestination != strings.Contains(got.Reply, "destination") {
					t.Fatalf("destination should only be requested when absent: %+v", got)
				}
			}
			if calls != 0 {
				t.Fatalf("vague request must not consume marketplace search quota: %d", calls)
			}
		})
	}
}

func TestReliableSupplierForConcreteProductKeepsSourcingQuery(t *testing.T) {
	post := "Looking for a reliable supplier for winter jackets shipped to UAE"
	if got := supplierQuery(post); got != "winter jackets" {
		t.Fatalf("specific product should remain sourceable, got %q", got)
	}
	if got := leadDestinationCountry(post); got != "AE" {
		t.Fatalf("UAE destination lost, got %q", got)
	}
}

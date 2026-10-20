package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/models"
)

func TestAuditMoneySubstringCannotAuthorizeExtraFee(t *testing.T) {
	for _, tc := range []struct{ price, freight, extra string }{
		{"120 CNY", "", "20 CNY"},
		{"120 CNY", "$114.20", "$14.20"},
	} {
		supplier := &models.SupplierMatch{Name: "hoodie", URL: "https://detail.1688.com/offer/123.html", PriceText: tc.price, Similar: true}
		reply := "A similar hoodie from China, reference price " + tc.price + ". "
		if tc.freight != "" {
			supplier.Shipping = &models.ShippingReference{PriceText: tc.freight}
			reply += "Reference shipping " + tc.freight + " per parcel. "
		}
		if !safeSupplierReply(reply+supplier.URL, "Need 20 pcs. Parcel volume 14.20", supplier, false) {
			t.Fatal("the original source-backed amounts must remain accepted")
		}
		reply += "Extra fee " + tc.extra + ". " + supplier.URL
		if safeSupplierReply(reply, "Need 20 pcs. Parcel volume 14.20", supplier, false) {
			t.Errorf("unsupported fee accepted as substring of a real price: %s", tc.extra)
		}
	}
}

func TestAuditServiceFallbackDoesNotInventCompanyCapabilities(t *testing.T) {
	for _, post := range []string{"Need a 3PL warehouse in the US for my Shopify store", "Bên nào có kho ở Mỹ nhận fulfill đơn Etsy không ạ?"} {
		got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 7, post, "Lan", nil, nil)
		if got.Reply == "" || strings.Contains(got.Reply, "THG") || strings.Contains(got.Reply, "bên mình có") || strings.Contains(got.Reply, "can help with warehousing") {
			t.Fatalf("fallback asserted an unverified company identity/capability: %+v", got)
		}
	}
}

func TestAuditTextOnlyVaguePostCannotInventProductCategory(t *testing.T) {
	const post = "Looking for a reliable supplier for winter products"
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.Sourcing, "winter gloves", post), "", &calls)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		t.Error("an invented category must not trigger sourcing")
		return nil, nil
	}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Lan", nil, nil, lookup)
	if got.Reply == "" || strings.Contains(got.Reply, "gloves") || !strings.Contains(got.Reply, "specific product") {
		t.Fatalf("a vague category was silently replaced with a guessed item: %+v", got)
	}
}

func TestAuditModelCannotSilenceExplicitProductSourcing(t *testing.T) {
	const post = "Looking for a supplier for hand massagers, 100 pcs to the US"
	for _, intent := range []string{leadcontext.NonCommercial, leadcontext.Shipping, leadcontext.Warehouse} {
		t.Run(intent, func(t *testing.T) {
			calls := 0
			gen := contextTestGenerator(t, testNeed(intent, "hand massagers", post), "", &calls)
			lookup := func(context.Context, string) (*ResolvedSupplier, error) {
				return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "hand massager", URL: "https://detail.1688.com/offer/123.html", Similar: true}}, nil
			}
			got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Lan", nil, nil, lookup)
			if !got.Supplier.HasOffer() || got.Reply == "" || got.ReplyNote == "" {
				t.Fatalf("tentative intent overrode explicit product need: %+v", got)
			}
		})
	}
}

func TestAuditPostBatteryWarningPreventsStandardCargo(t *testing.T) {
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		weight := 0.8
		return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "Áo hoodie", URL: "https://detail.1688.com/offer/123.html", PriceText: "20 CNY"}, WeightKG: &weight}, nil
	}
	quote := func(_ context.Context, req models.ShippingRequest) (*models.ShippingReference, error) {
		if req.CargoCategory == "standard" {
			t.Error("explicit battery warning was ignored because the supplier title was generic")
			return &models.ShippingReference{PriceText: "$14.20"}, nil
		}
		return nil, nil
	}
	got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 7, "Cần mua 1 áo hoodie dùng pin đi Mỹ", "Lan", quote, lookup)
	if !got.Supplier.HasOffer() || got.Supplier.Shipping != nil || strings.Contains(got.Reply, "$14.20") {
		t.Fatalf("unsafe standard-cargo reference leaked: %+v", got)
	}
}

func TestAuditUnreadablePhotoCannotBeReportedAsRead(t *testing.T) {
	const post = "Looking for a supplier for hand massagers"
	need := testNeed(leadcontext.Sourcing, "hand massagers", post)
	need.Visual.Uncertainties = []string{"The photo cannot be read"}
	calls := 0
	gen := contextTestGenerator(t, need, "", &calls)
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Lan", []string{"https://scontent.fbcdn.net/post.jpg"}, nil, nil)
	if got.Reply == "" || !strings.Contains(got.ContextNote, "Chưa") {
		t.Fatalf("missing image evidence was presented as successful reading: %+v", got)
	}
}

func TestAuditAdvertCannotBeTurnedIntoBuyerByModel(t *testing.T) {
	const post = "We offer hand massagers and provide sourcing services worldwide"
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.Sourcing, "hand massagers", post), "", &calls)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		t.Error("an obvious service offer must not spend sourcing requests")
		return nil, nil
	}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Seller", nil, nil, lookup)
	if got.Reply != "" || got.ReplyNote == "" || calls != 0 {
		t.Fatalf("advertisement became a buyer request: %+v calls=%d", got, calls)
	}
}

func TestAuditRealShippingRequestIsNotOverriddenByProductWords(t *testing.T) {
	const post = "Need a shipping agent for cotton bags to UAE, 200kg per month"
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.Shipping, "cotton bags", post), "", &calls)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		t.Error("a service-only request must not search for merchandise")
		return nil, nil
	}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Lan", nil, nil, lookup)
	if got.AnalysisStatus != leadcontext.Shipping || got.Reply == "" || got.Supplier.HasOffer() {
		t.Fatalf("legitimate service intent was changed: %+v", got)
	}
}

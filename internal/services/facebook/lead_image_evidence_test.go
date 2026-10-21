package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/models"
)

func imageRequestNeed() leadcontext.Context {
	need := testNeed(leadcontext.Sourcing, "hand massager", "")
	need.Evidence = []string{}
	need.Visual.Product = "hand massager"
	need.Visual.Observations = []string{"Visible request: Need a supplier for hand massagers, 300 pcs to the US"}
	need.ImageEvidence = []leadcontext.ImageEvidence{{ImageIndex: 0, Quote: "Need a supplier for hand massagers, 300 pcs to the US"}}
	return need
}

func TestImageRequestFindsSourceWithoutInventingQuoteInputs(t *testing.T) {
	calls, lookups := 0, 0
	gen := contextTestGenerator(t, imageRequestNeed(), "", &calls)
	lookup := func(_ context.Context, text string) (*ResolvedSupplier, error) {
		lookups++
		if supplierQuery(text) != "hand massager" || leadQuantity(text) != 0 || leadDestinationCountry(text) != "" {
			t.Error("image quotation was promoted to authoritative shipment inputs")
		}
		weight := 0.4
		return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "hand massager", Similar: true,
			URL: "https://detail.1688.com/offer/123.html", PriceText: "20 CNY"}, WeightKG: &weight}, nil
	}
	quote := func(context.Context, models.ShippingRequest) (*models.ShippingReference, error) {
		t.Error("unverified image-only quantity/destination reached rate router")
		return nil, nil
	}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7,
		"Please refer to the attached request", "Sample", []string{"https://scontent.fbcdn.net/synthetic.jpg"}, quote, lookup)
	if calls != 1 || lookups != 1 || !got.Supplier.HasOffer() || !strings.Contains(got.Reply, got.Supplier.URL) || got.Supplier.Shipping != nil {
		t.Fatalf("image request lost grounded source: calls=%d lookups=%d analysis=%s note=%s sourcing=%s query=%q", calls, lookups, got.AnalysisStatus, got.ReplyNote, got.SourcingNote, supplierQuery(leadContextSearchText("Please refer to the attached request", imageRequestNeed())))
	}
}

func TestImageRequestCannotReferenceUnsubmittedPhoto(t *testing.T) {
	need := imageRequestNeed()
	need.ImageEvidence[0].ImageIndex = 1
	calls := 0
	gen := contextTestGenerator(t, need, "", &calls)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		t.Error("invalid image evidence reached sourcing")
		return nil, nil
	}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7,
		"Please refer to the attached request", "Sample", []string{"https://scontent.fbcdn.net/synthetic.jpg"}, nil, lookup)
	if calls != 1 || got.AnalysisStatus != "unavailable" || got.ReplyNote == "" {
		t.Fatal("invalid image evidence did not take explained fallback")
	}
}

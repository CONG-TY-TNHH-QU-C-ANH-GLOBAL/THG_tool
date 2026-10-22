package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/models"
)

func TestScopeQuestionStopsLookupAndDoesNotRepeatKnownQuantity(t *testing.T) {
	for _, suffix := range []string{"", ", 50 cái", ", 50 cuộn", ", 50 rolls", ", 50 skeins"} {
		post := "Mình cần tìm nguồn len như mẫu" + suffix
		need := testNeed(leadcontext.Sourcing, "", post)
		need.Visual.Product = "giỏ"
		need.Visual.Observations = []string{"Giỏ màu kem có tên riêng"}
		need.Scope = &leadcontext.ScopeClarification{Kind: "purchase_scope", Alternatives: []string{"len", "giỏ"}}
		calls := 0
		gen := contextTestGenerator(t, need, "", &calls)
		lookup := func(context.Context, string) (*ResolvedSupplier, error) {
			t.Error("ambiguous purchase must not search marketplaces")
			return nil, nil
		}
		shipping := func(context.Context, models.ShippingRequest) (*models.ShippingReference, error) {
			t.Error("ambiguous purchase must not obtain rates")
			return nil, nil
		}
		got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "", []string{"https://scontent.fbcdn.net/reference.jpg"}, shipping, lookup)
		if calls != 1 || got.AnalysisStatus != "needs_scope_confirmation" || !strings.Contains(got.Reply, "len hay giỏ") {
			t.Fatalf("missing focused question: %+v; calls=%d", got, calls)
		}
		if strings.Contains(got.Reply, "số lượng") != (suffix == "") || got.ProductURL != "" || got.Supplier.HasOffer() || got.SourcingNote != "" {
			t.Fatalf("question repeats facts or invents an offer: %+v", got)
		}
		if got.ImageInputs != 1 || got.ImageAnalysis != "observations" {
			t.Fatal("clarification lost image audit")
		}
	}
}

func TestTextOnlyScopeQuestionKeepsEnglishAndDoesNotInventPhoto(t *testing.T) {
	post := "Need thread or a storage basket"
	need := testNeed(leadcontext.Sourcing, "", post)
	need.Scope = &leadcontext.ScopeClarification{Kind: "purchase_scope", Alternatives: []string{"thread", "storage basket"}}
	calls := 0
	gen := contextTestGenerator(t, need, "", &calls)
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "", nil, nil, nil)
	if calls != 1 || !strings.HasPrefix(got.Reply, "Are you looking for thread or storage basket?") || got.ImageAnalysis != "none" {
		t.Fatalf("text-only question changed language/media: %+v", got)
	}
}

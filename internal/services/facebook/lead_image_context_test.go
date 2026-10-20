package facebook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/visualcontext"
	"github.com/thg/scraper/internal/models"
)

func imageTestGenerator(t *testing.T, content string, status int) *ai.MessageGenerator {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	t.Cleanup(server.Close)
	return ai.NewMessageGeneratorWithEndpoint("fake-key", "configured-model", server.URL)
}

const imageTestContext = `{"product":"hand massager","observations":["Device visible"],"uncertainties":["Model unreadable"],"text_image_conflict":false}`
const imageTestURL = "https://scontent.fbcdn.net/original.jpg"

func TestImageContextCanSupplyMissingGenericProduct(t *testing.T) {
	lookupCalls := 0
	lookup := func(_ context.Context, text string) (*ResolvedSupplier, error) {
		lookupCalls++
		if supplierQuery(text) != "hand massager" || leadQuantity(text) != 100 || leadDestinationCountry(text) != "US" {
			t.Errorf("lost image hint or original request: %q", text)
		}
		return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "hand massager", URL: "https://detail.1688.com/offer/123.html", Similar: true}}, nil
	}
	post := "Looking for a supplier for this model, 100 pcs to the US"
	got := BuildLeadSuggestionWithImages(context.Background(), nil, imageTestGenerator(t, imageTestContext, 200), nil, 7,
		post, "Lan", []string{imageTestURL}, nil, lookup)
	if lookupCalls != 1 || !got.Supplier.HasOffer() || !got.Supplier.Similar || got.ContextNote == "" || got.Reply == "" {
		t.Fatalf("image-aware sourcing did not preserve similar status: %+v, calls=%d", got, lookupCalls)
	}
	if got.Supplier.WeightKG != "" || got.Supplier.PriceText != "" || got.Supplier.Shipping != nil {
		t.Fatal("image observations must not turn into supplier price or transport facts")
	}
}

func TestConflictingImagesAskBeforeLookup(t *testing.T) {
	conflict := strings.Replace(imageTestContext, `"text_image_conflict":false`, `"text_image_conflict":true`, 1)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		t.Error("conflicting text/images must not trigger sourcing")
		return nil, nil
	}
	got := BuildLeadSuggestionWithImages(context.Background(), nil, imageTestGenerator(t, conflict, 200), nil, 7,
		"Need cotton bags, 100 pcs to the US", "Lan", []string{imageTestURL}, nil, lookup)
	if got.Supplier != nil || got.ProductURL != "" || got.Reply == "" || !strings.Contains(got.ContextNote, "chưa khớp") {
		t.Fatalf("conflict must remain a clarification: %+v", got)
	}
}

func TestFailedImageContextKeepsTextDraft(t *testing.T) {
	got := BuildLeadSuggestionWithImages(context.Background(), nil, imageTestGenerator(t, "PRIVATE_PROVIDER_ERROR", 400), nil, 7,
		"Looking for a reliable supplier for winter products", "Lan", []string{imageTestURL}, nil, nil)
	if got.Reply == "" || !strings.Contains(got.ContextNote, "Chưa phân tích") || strings.Contains(got.ContextNote, "PRIVATE") {
		t.Fatalf("image failure must not remove text fallback or expose provider errors: %+v", got)
	}
}

func TestImageHintNeverOverridesExplicitTextOrService(t *testing.T) {
	visual := []visualcontext.Context{{Product: "hand massager"}}
	for _, post := range []string{"Need custom logo printing on ceramic mugs", "Looking for a shipping agent to USA", "We offer sourcing services", "Need this https://detail.1688.com/offer/123.html"} {
		if got := imageAwareLeadText(post, visual); got != post {
			t.Errorf("image changed an explicit requirement or service: %q", got)
		}
	}
}

func TestMissingOriginalPhotoDoesNotPretendItWasRead(t *testing.T) {
	got := BuildLeadSuggestionWithImages(context.Background(), nil, nil, nil, 7,
		"Looking for a supplier for this model, 100 pcs to the US", "Lan", []string{"https://supplier.example/catalog.jpg"}, nil, nil)
	if got.Reply == "" || !strings.Contains(got.ContextNote, "Chưa nhận") || got.Supplier.HasOffer() {
		t.Fatalf("missing original image must retain a truthful text draft: %+v", got)
	}
}

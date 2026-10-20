package facebook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestAnalysisBudgetIsExplicitlyScoped(t *testing.T) {
	scope := ParseOrgAllowlist("7")
	for _, tc := range []struct {
		enabled    bool
		org        int64
		base, want time.Duration
	}{
		{true, 7, 12 * time.Second, 24 * time.Second},
		{true, 8, 12 * time.Second, 12 * time.Second},
		{false, 7, 12 * time.Second, 12 * time.Second},
		{true, 7, 30 * time.Second, 30 * time.Second},
	} {
		if got := LeadSuggestionTimeout(tc.base, tc.enabled, scope, tc.org); got != tc.want {
			t.Errorf("unexpected budget for org %d: %v", tc.org, got)
		}
	}
}

func TestAnalysisLatencyDoesNotStarveSourcing(t *testing.T) {
	const post = "Looking for a supplier for hand massagers from 1688"
	const link = "https://detail.1688.com/offer/123.html"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(3300 * time.Millisecond)
		data, _ := json.Marshal(testNeed(leadcontext.Sourcing, "hand massagers", post))
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(data)}}}})
	}))
	defer server.Close()
	gen := ai.NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
	for _, tc := range []struct {
		name      string
		timeout   time.Duration
		wantOffer bool
	}{
		{"legacy twelve-second budget reproduces starvation", 12 * time.Second, false},
		{"scoped analysis budget retains sourcing", LeadSuggestionTimeout(12*time.Second, true, ParseOrgAllowlist("7"), 7), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			price := 20.0
			client := &fakeSupplierReader{
				items:   []suppliersourcing.SearchItem{{ID: "123", Title: "hand massagers", Link: link}},
				product: &suppliersourcing.Product{Title: "hand massagers", Link: link, Price: &price},
			}
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()
			got := BuildLeadSuggestionWithContext(ctx, nil, gen, nil, 7, post, "Lan", nil, nil, NewSupplierLookup(client))
			if got.Supplier.HasOffer() != tc.wantOffer || got.Reply == "" || got.AnalysisStatus != leadcontext.Sourcing {
				t.Fatalf("latency/sourcing budget mismatch: %+v requests=%v", got, client.queries)
			}
			if tc.wantOffer && !containsExactReplyURL(got.Reply, link) {
				t.Fatal("sourced URL lost")
			}
		})
	}
}

func TestSlowWordingKeepsGroundedOfferBeforeOuterDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer server.Close()
	gen := ai.NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
	const link = "https://detail.1688.com/offer/123.html"
	const draft = "Similar hoodie from China, reference price 20 CNY. " + link
	out := LeadSuggestion{Reply: draft, Supplier: &models.SupplierMatch{Name: "hoodie", URL: link, PriceText: "20 CNY", Similar: true}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := refineLeadSuggestion(ctx, nil, gen, &ai.BusinessProfile{Name: "THG"}, 7, "Need a hoodie", "Lan", out, "{}", "")
	if ctx.Err() != nil || got.Reply != draft || got.ReplyStatus != "model_error" || !strings.Contains(got.Reply, link) {
		t.Fatalf("optional wording erased the grounded offer or exhausted runner deadline: %+v, err=%v", got, ctx.Err())
	}
}

package facebook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

type podFallbackSearcher struct{ hit retrieval.Hit }

func (s podFallbackSearcher) TopK(context.Context, int64, string, retrieval.SearchFilter, int) ([]retrieval.Hit, error) {
	return []retrieval.Hit{s.hit}, nil
}

func (s podFallbackSearcher) TopKWithTrace(context.Context, int64, string, retrieval.SearchFilter, int) ([]retrieval.Hit, retrieval.Trace, error) {
	return []retrieval.Hit{s.hit}, retrieval.Trace{}, nil
}

func TestPODSuggestionKeepsDraftWhenGeneratorFails(t *testing.T) {
	const catalogURL = "https://thgfulfill.com/vi/catalog?productId=cup-1"
	for _, tc := range []struct {
		name, post, author, title, response string
		status                              int
		wantGreeting                        string
	}{
		{"provider error", "Cần in logo lên cốc sứ", "Lan", "Cốc sứ in logo", `{"error":"unavailable"}`, 400, "Lan,"},
		{"blank provider output", "Cần in logo lên cốc sứ", "Lan", "Cốc sứ in logo", `{"choices":[{"message":{"content":"   "}}]}`, 200, "Lan,"},
		{"English without author", "Need custom logo printing on ceramic mugs", "", "Ceramic mugs custom", `{"error":"unavailable"}`, 400, "Hi there,"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			builder := &knowledgeRuntime.Builder{Searcher: podFallbackSearcher{hit: retrieval.Hit{Asset: &assets.Asset{
				Type: assets.AssetPODProduct, State: assets.StateApproved, Title: tc.title,
				Payload: []byte(`{"source_url":"` + catalogURL + `","availability":"in_stock"}`),
			}}}}
			generator := ai.NewMessageGeneratorWithEndpoint("test-key", "test-model", server.URL)
			got := BuildLeadSuggestion(context.Background(), builder, generator, &ai.BusinessProfile{Name: "THG"}, 1, tc.post, tc.author, nil, nil)
			if calls == 0 || got.ProductURL != catalogURL || got.Supplier != nil {
				t.Fatalf("expected matched POD and generator call, got %+v, calls=%d", got, calls)
			}
			if !strings.HasPrefix(got.Reply, tc.wantGreeting) || !strings.Contains(got.Reply, tc.title) || !strings.Contains(got.Reply, catalogURL) {
				t.Fatalf("missing grounded fallback draft: %q", got.Reply)
			}
		})
	}
}

package facebook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/leadcontext"
)

func TestCleanPostAndValidatedPhotosShareOneModelRequest(t *testing.T) {
	const body = "Need 200 custom mugs shipped to Canada"
	const raw = "Facebook Sample Author · Follow · 2 hrs ago · " + body + " Xem bản dịch 3 2 Seller · 4 phút · Theo dõi We have stock and ship from the US"
	photos := []string{"https://scontent.fbcdn.net/one.jpg?token=PRIVATE_REFERENCE", "https://scontent.fbcdn.net/two.jpg", "https://scontent.fbcdn.net/three.jpg"}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			Messages []struct {
				Content []struct {
					Type     string               `json:"type"`
					Text     string               `json:"text"`
					ImageURL struct{ URL string } `json:"image_url"`
				}
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 1 || len(request.Messages[0].Content) != 4 {
			t.Error("expected one request containing text and three admitted photos")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		parts := request.Messages[0].Content
		_, data, found := strings.Cut(parts[0].Text, "INPUT_DATA:\n")
		var input struct{ Post string }
		if parts[0].Type != "text" || !found || json.Unmarshal([]byte(data), &input) != nil || input.Post != body {
			t.Errorf("author content changed or UI/comments reached model: %q", input.Post)
		}
		for i, photo := range photos {
			if parts[i+1].Type != "image_url" || parts[i+1].ImageURL.URL != photo {
				t.Error("photo association/order or signed query changed")
			}
		}
		need, _ := json.Marshal(testNeed(leadcontext.Unclear, "", body))
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(need)}}}})
	}))
	defer server.Close()
	gen := ai.NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
	rawPhotos := []string{photos[0], "https://localhost/private.jpg", photos[0], photos[1], photos[2], "https://scontent.fbcdn.net/fourth.jpg"}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, raw, "Sample Author", rawPhotos, nil, nil)
	if calls != 1 || got.ImageInputs != 3 || got.AnalysisStatus != leadcontext.Unclear {
		t.Fatalf("input did not reach analysis as one bounded request: calls=%d result=%+v", calls, got)
	}
}

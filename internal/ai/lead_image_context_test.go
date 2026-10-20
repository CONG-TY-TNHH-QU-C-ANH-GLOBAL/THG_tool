package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnalyzeLeadImagesSendsTextAndPhotosTogether(t *testing.T) {
	const photo = "https://scontent.fbcdn.net/original.jpg?token=private"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string                               `json:"model"`
			Messages []struct{ Content []map[string]any } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		parts := body.Messages[0].Content
		if body.Model != "configured-model" || len(parts) != 2 || !strings.Contains(parts[0]["text"].(string), "100 pcs to US") {
			t.Error("text and image context lost")
		}
		if parts[1]["image_url"].(map[string]any)["url"] != photo {
			t.Error("wrong original photo reference")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{
			"content": `{"product":"hand massager","observations":["Device visible"],"uncertainties":["Model unclear"],"text_image_conflict":false}`,
		}}}})
	}))
	defer server.Close()
	generator := NewMessageGeneratorWithEndpoint("fake-key", "configured-model", server.URL)
	got, err := generator.AnalyzeLeadImages(context.Background(), "Need this, 100 pcs to US", []string{photo})
	if err != nil || got.Product != "hand massager" {
		t.Fatalf("image context missing: %+v, %v", got, err)
	}
}

func TestAnalyzeLeadImagesDoesNotExposeProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("PRIVATE_SIGNED_URL_OR_KEY"))
	}))
	defer server.Close()
	generator := NewMessageGeneratorWithEndpoint("fake-key", "configured-model", server.URL)
	_, err := generator.AnalyzeLeadImages(context.Background(), "Need this", []string{"https://scontent.fbcdn.net/p.jpg"})
	if err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatalf("provider failure not safely surfaced: %v", err)
	}
}

package facebook

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/models"
)

func TestUnifiedImageAuditMatchesActualRequestAndFallback(t *testing.T) {
	const caption = "mình cần tìm bên fulfill sản phẩm này ạ"
	const photo = "https://scontent.fbcdn.net/post.jpg?token=PRIVATE_REFERENCE"
	for _, tc := range []struct {
		name, status string
		images       []string
		observations []string
		conflict     bool
		fail         bool
		inputs       int
	}{
		{name: "text only", status: "none"},
		{name: "untrusted reference", status: "references_rejected", images: []string{"https://localhost/private.jpg"}},
		{name: "model observations", status: "observations", images: []string{photo}, observations: []string{"Hoa và tên Isabella hiển thị trên sản phẩm"}, inputs: 1},
		{name: "empty observations", status: "no_observations", images: []string{photo}, inputs: 1},
		{name: "conflict early return", status: "conflict", images: []string{photo}, observations: []string{"Ảnh thể hiện mẫu khác"}, conflict: true, inputs: 1},
		{name: "provider failed fallback", status: "analysis_failed", images: []string{photo}, fail: true, inputs: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			need := testNeed(leadcontext.Sourcing, "", caption)
			need.Visual.Conflict = tc.conflict
			if len(tc.observations) > 0 {
				need.Visual.Observations = tc.observations
			}
			var actualImages int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actualImages += countAuditRequestImages(t, r)
				if tc.fail {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte("PRIVATE_PROVIDER_RESPONSE"))
					return
				}
				data, _ := json.Marshal(need)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(data)}}}})
			}))
			defer server.Close()
			gen := ai.NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
			out := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, caption, "Lan", tc.images, nil, nil)
			if actualImages != tc.inputs || out.ImageInputs != actualImages || out.ImageAnalysis != tc.status {
				t.Fatalf("request/audit mismatch: actual=%d audit=%+v", actualImages, out)
			}
			assertImageAuditLogPrivate(t, out, len(tc.images))
		})
	}
}

func countAuditRequestImages(t *testing.T, r *http.Request) int {
	t.Helper()
	var body struct {
		Messages []struct{ Content []map[string]any }
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 1 {
		t.Error("invalid model request")
		return 0
	}
	count := 0
	for _, part := range body.Messages[0].Content {
		if part["type"] == "image_url" {
			count++
		}
	}
	return count
}

func assertImageAuditLogPrivate(t *testing.T, out models.LeadSuggestion, eventImages int) {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(previous)
	LogLeadSuggestionOutcome("crawl", 7, 42, SuggestionGateAttempted, out, eventImages)
	if strings.Contains(buf.String(), "PRIVATE") || strings.Contains(buf.String(), "fbcdn") || strings.Contains(buf.String(), "Isabella") {
		t.Fatal("image URL, provider body or observations leaked to log")
	}
}

func TestImageAuditLogRejectsUntrustedStatus(t *testing.T) {
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(previous)
	LogLeadSuggestionOutcome("crawl", 7, 42, SuggestionGateAttempted,
		models.LeadSuggestion{ImageInputs: 9, ImageAnalysis: "https://private.example/photo?secret=PRIVATE"}, 9)
	for _, token := range []string{"event_images=3", "model_images=3", "image_analysis=not_recorded"} {
		if !strings.Contains(buf.String(), token) {
			t.Fatalf("missing safe audit field %q", token)
		}
	}
	if strings.Contains(buf.String(), "PRIVATE") {
		t.Fatal("untrusted audit status leaked")
	}
}

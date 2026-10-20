package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLeadReplyRejectsTruncationAndDoesNotExposeProviderErrors(t *testing.T) {
	for _, mode := range []string{"length", "refusal", "provider_error"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if mode == "provider_error" {
					w.WriteHeader(503)
					_, _ = w.Write([]byte("PRIVATE_ECHOED_INPUT_AND_KEY"))
					return
				}
				message := map[string]string{"content": "Partial draft"}
				if mode == "refusal" {
					message["refusal"] = "PRIVATE_REFUSAL"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message, "finish_reason": mode}}})
			}))
			defer server.Close()
			gen := NewMessageGeneratorWithEndpoint("fake-key", "muse-spark-1.3", server.URL)
			reply, err := gen.GenerateLeadReplySuggestion(context.Background(), LeadReplyRequest{PostContent: "Need shipping"})
			if err == nil || reply != "" || calls != 1 || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("provider failure was not safely bounded: reply=%q calls=%d err=%v", reply, calls, err)
			}
		})
	}
}

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLeadAnalysisFailureCodesDoNotExposeProviderData(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
		status              int
	}{
		{"input rejected", "PRIVATE_SIGNED_URL", "provider_rejected_input", 400},
		{"credentials", "PRIVATE_API_KEY", "provider_auth", 401},
		{"rate limit", "PRIVATE_ACCOUNT", "provider_rate_limit", 429},
		{"provider error", "PRIVATE_PROVIDER_BODY", "provider_http_error", 503},
		{"bad envelope", "PRIVATE_MALFORMED_JSON", "invalid_response", 200},
		{"invalid interpretation", "{}", "context_validation", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.content))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": tc.content}}}})
			}))
			defer server.Close()
			gen := NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
			_, err := gen.AnalyzeLeadContext(context.Background(), "Need this product", nil)
			if err == nil || LeadModelFailureCode(err) != tc.want || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("wrong/unsafe failure code: code=%s err=%v", LeadModelFailureCode(err), err)
			}
		})
	}
	if LeadModelFailureCode(errors.New("PRIVATE_TRANSPORT_ERROR")) != "unclassified" {
		t.Fatal("unknown errors must not be passed through")
	}
}

func TestLeadAnalysisExpiredContextIsReportedWithoutRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	gen := NewMessageGeneratorWithEndpoint("fake-key", "test-model", server.URL)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := gen.AnalyzeLeadContext(ctx, "Need this product", nil)
	if LeadModelFailureCode(err) != "timeout" || calls != 0 {
		t.Fatalf("expired budget was not respected: code=%s calls=%d", LeadModelFailureCode(err), calls)
	}
}

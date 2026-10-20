package config

import "testing"

func TestLeadModelOverrideNeverBorrowsCommentCredentials(t *testing.T) {
	t.Setenv("LLM_COMMENT_API_KEY", "comment-test-key")
	t.Setenv("LLM_COMMENT_BASE_URL", "https://comment.example/v1")
	t.Setenv("OPENAI_COMMENT_MODEL", "comment-model")
	t.Setenv("LEAD_MODEL_API_KEY", "")
	t.Setenv("LEAD_MODEL_BASE_URL", "https://api.meta.ai/v1/")
	t.Setenv("LEAD_MODEL_NAME", "muse-spark-1.3")
	cfg := Load()
	if !cfg.LeadModel.Explicit() || cfg.LeadModel.Complete() || cfg.LeadModel.APIKey != "" {
		t.Fatal("partial lead override must stay unavailable, without borrowed credentials")
	}
	t.Setenv("LEAD_MODEL_API_KEY", "lead-test-key")
	cfg = Load()
	if !cfg.LeadModel.Complete() || cfg.LeadModel.BaseURL != "https://api.meta.ai/v1" || cfg.LLMCommentAPIKey != "comment-test-key" || cfg.OpenAICommentModel != "comment-model" {
		t.Fatal("lead provider must be isolated from outbound comments")
	}
}

func TestLeadModelEndpointRejectsCredentialsAndNonHTTPS(t *testing.T) {
	for _, base := range []string{"http://api.example/v1", "https://user:pass@api.example/v1", "https://api.example/v1?key=secret", "https://api.example/v1#fragment", "not-a-url"} {
		if (LeadModelConfig{APIKey: "test-key", BaseURL: base, Model: "model"}).Complete() {
			t.Fatal("unsafe explicit model endpoint accepted")
		}
	}
}

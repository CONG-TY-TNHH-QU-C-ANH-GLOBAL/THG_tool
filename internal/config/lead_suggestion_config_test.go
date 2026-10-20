package config

import "testing"

func TestLoadLeadSuggestionDefaultsFailClosed(t *testing.T) {
	t.Setenv("LEAD_SUGGESTION_ENABLED", "")
	t.Setenv("LEAD_IMAGE_CONTEXT_ENABLED", "")
	t.Setenv("LEAD_CONTEXT_ANALYSIS_ENABLED", "")
	t.Setenv("LEAD_SUGGESTION_ORG_IDS", "")
	t.Setenv("LEAD_SUGGESTION_TIMEOUT_MS", "")
	t.Setenv("LEAD_SUGGESTION_MAX_CONCURRENCY", "")
	cfg := Load()
	if cfg.LeadSuggestionEnabled || cfg.LeadImageContextEnabled || cfg.LeadContextAnalysisEnabled || cfg.LeadSuggestionOrgIDs != "" {
		t.Fatal("suggestions must default off with no allowed orgs")
	}
	if cfg.LeadSuggestionTimeoutMS != 12000 || cfg.LeadSuggestionMaxConcurrency != 2 {
		t.Fatalf("unexpected bounded-runner defaults: timeout=%d concurrency=%d", cfg.LeadSuggestionTimeoutMS, cfg.LeadSuggestionMaxConcurrency)
	}
}

func TestLoadLeadSuggestionCanaryConfig(t *testing.T) {
	t.Setenv("LEAD_SUGGESTION_ENABLED", "true")
	t.Setenv("LEAD_IMAGE_CONTEXT_ENABLED", "true")
	t.Setenv("LEAD_CONTEXT_ANALYSIS_ENABLED", "true")
	t.Setenv("LEAD_SUGGESTION_ORG_IDS", "7")
	t.Setenv("LEAD_SUGGESTION_TIMEOUT_MS", "2500")
	t.Setenv("LEAD_SUGGESTION_MAX_CONCURRENCY", "1")
	cfg := Load()
	if !cfg.LeadSuggestionEnabled || !cfg.LeadImageContextEnabled || !cfg.LeadContextAnalysisEnabled || cfg.LeadSuggestionOrgIDs != "7" {
		t.Fatal("canary config not loaded")
	}
	if cfg.LeadSuggestionTimeoutMS != 2500 || cfg.LeadSuggestionMaxConcurrency != 1 {
		t.Fatalf("runner config not loaded: timeout=%d concurrency=%d", cfg.LeadSuggestionTimeoutMS, cfg.LeadSuggestionMaxConcurrency)
	}
}

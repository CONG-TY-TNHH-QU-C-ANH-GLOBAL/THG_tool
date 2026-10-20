package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// This is a generator preview, not a production pipeline or knowledge import.
// Only synthetic posts and public facts may be sent to the configured provider.
func liveReply(ctx context.Context, dir, query, author, envFile string, review, publicOnly bool) error {
	if !review || !publicOnly || strings.TrimSpace(query) == "" {
		return errors.New("live preview requires a synthetic -query, -review-draft and -public-facts-only")
	}
	preview, err := preview(ctx, dir, query, true, true, 4)
	if err != nil {
		return err
	}
	if len(preview.Hits) == 0 {
		return errors.New("no public facts retrieved; no provider request made")
	}
	generator, model, err := previewModel(envFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	reply, err := generator.GenerateLeadReplySuggestion(ctx, previewReplyRequest(query, author, preview.Hits))
	if err != nil || strings.TrimSpace(reply) == "" {
		// Provider errors may contain URLs, echoed input or credentials. Never
		// serialize them into a shared local report.
		return errors.New("model request failed or returned an empty reply; no outbound message sent")
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Mode           string       `json:"mode"`
		Model          string       `json:"model"`
		RuntimeEnabled bool         `json:"runtime_enabled"`
		Facts          []previewHit `json:"retrieved_public_facts"`
		Reply          string       `json:"actual_model_reply_unreviewed"`
		Review         string       `json:"review_required"`
	}{"live_model_draft_review", model, false, preview.Hits, reply,
		"Check intent, unsupported claims, unnecessary questions, language and naturalness. This is not an accepted production reply."})
}

func previewReplyRequest(query, author string, hits []previewHit) ai.LeadReplyRequest {
	var facts []assets.ReplyKnowledgeV1
	for _, hit := range hits {
		if hit.Audience != "public_reference" || hit.Verification != "public_source_checked" {
			continue
		}
		var ids []string
		for _, source := range hit.Sources {
			ids = append(ids, source.ID)
		}
		facts = append(facts, assets.ReplyKnowledgeV1{Schema: "company_reply_v1", Audience: hit.Audience,
			Verification: hit.Verification, Facts: hit.Facts, SourceIDs: ids})
	}
	data, _ := json.Marshal(facts)
	return ai.LeadReplyRequest{PostContent: query, AuthorName: author, BrandName: "THG",
		CompanyKnowledge: string(data), BusinessContext: "THG — synthetic local review. No operational commitments approved."}
}

func previewModel(envFile string) (*ai.MessageGenerator, string, error) {
	values := map[string]string{}
	if envFile != "" {
		var err error
		values, err = godotenv.Read(envFile)
		if err != nil {
			return nil, "", errors.New("cannot read private model config; contents were not logged")
		}
	}
	get := func(names ...string) string {
		return previewModelValue(values, names...)
	}
	lead := config.LeadModelConfig{APIKey: get("LEAD_MODEL_API_KEY"), BaseURL: strings.TrimRight(get("LEAD_MODEL_BASE_URL"), "/"), Model: get("LEAD_MODEL_NAME")}
	if lead.Explicit() {
		if !lead.Complete() {
			return nil, "", errors.New("explicit lead model config requires LEAD_MODEL_API_KEY, HTTPS LEAD_MODEL_BASE_URL and LEAD_MODEL_NAME; no comment credentials were reused")
		}
		return ai.NewMessageGeneratorWithEndpoint(lead.APIKey, lead.Model, lead.BaseURL), lead.Model, nil
	}
	key := get("LLM_COMMENT_API_KEY", "LLM_API_KEY", "OPENAI_API_KEY")
	if key == "" {
		return nil, "", errors.New("model credential not configured; set LLM_COMMENT_API_KEY privately, never paste it in chat")
	}
	model := get("OPENAI_COMMENT_MODEL")
	if model == "" {
		model = "gpt-4.1"
	}
	endpoint := get("LLM_COMMENT_BASE_URL", "LLM_BASE_URL")
	return ai.NewMessageGeneratorWithEndpoint(key, model, endpoint), model, nil
}

func previewModelValue(values map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
		if value := strings.TrimSpace(values[name]); value != "" {
			return value
		}
	}
	return ""
}

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/ai/visualcontext"
)

// AnalyzeLeadImages uses this generator's provider/model, with one bounded
// request. Callers validate platform image URLs before invoking it. No media is
// fetched on the application server and no provider error body is logged.
func (mg *MessageGenerator) AnalyzeLeadImages(ctx context.Context, post string, urls []string) (visualcontext.Context, error) {
	if len(urls) == 0 {
		return visualcontext.Context{}, errors.New("image context unavailable")
	}
	content, err := mg.requestLeadAnalysis(ctx, visualcontext.Prompt(sanitizeForPrompt(post, 6000)), urls)
	if err != nil {
		return visualcontext.Context{}, err
	}
	return visualcontext.Parse([]byte(content))
}

// AnalyzeLeadContext analyzes text-only posts too. Its result is tentative;
// quantities, destinations and supplier facts remain owned by the source path.
func (mg *MessageGenerator) AnalyzeLeadContext(ctx context.Context, post string, urls []string) (leadcontext.Context, error) {
	post = sanitizeForPrompt(post, 6000)
	content, err := mg.requestLeadAnalysis(ctx, leadcontext.Prompt(post), urls)
	if err != nil {
		return leadcontext.Context{}, err
	}
	return leadcontext.Parse([]byte(content), post, len(urls) > 0)
}

func (mg *MessageGenerator) requestLeadAnalysis(ctx context.Context, prompt string, urls []string) (string, error) {
	if mg == nil || !mg.Available() || len(urls) > 3 {
		return "", errors.New("lead analysis unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	body := chatCompletionBody(mg.model, "")
	parts := []map[string]any{{"type": "text", "text": prompt}}
	for _, imageURL := range urls {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": imageURL, "detail": "auto"}})
	}
	body["messages"] = []map[string]any{{"role": "user", "content": parts}}
	body["response_format"] = map[string]string{"type": "json_object"}
	return mg.requestLeadCompletion(ctx, body)
}

// requestLeadCompletion is shared only by operator analysis/reply generation.
// It does not retry, expose provider bodies, or accept truncated completions.
func (mg *MessageGenerator) requestLeadCompletion(ctx context.Context, body map[string]any) (string, error) {
	if mg == nil || !mg.Available() {
		return "", errors.New("lead model unavailable")
	}
	mg.applyProviderTuning(body)
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mg.chatURL(), bytes.NewReader(data))
	if err != nil {
		return "", errors.New("invalid lead analysis request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mg.apiKey)
	response, err := mg.client.Do(req)
	if err != nil {
		return "", errors.New("lead analysis request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("lead analysis provider unavailable")
	}
	var result struct {
		Choices []struct {
			Message      struct{ Content, Refusal string } `json:"message"`
			FinishReason string                            `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 32768)).Decode(&result) != nil || len(result.Choices) != 1 {
		return "", errors.New("invalid lead analysis response")
	}
	choice := result.Choices[0]
	if choice.FinishReason == "length" || choice.Message.Refusal != "" || strings.TrimSpace(choice.Message.Content) == "" {
		return "", errors.New("incomplete lead analysis response")
	}
	return choice.Message.Content, nil
}

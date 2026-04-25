package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SelectorHealer uses GPT-4o Vision to discover working CSS selectors
// when the existing selector cache fails (e.g., after a Facebook UI change).
type SelectorHealer struct {
	apiKey string
	model  string
}

// NewSelectorHealer creates a healer backed by GPT-4o Vision.
func NewSelectorHealer(apiKey string) *SelectorHealer {
	return &SelectorHealer{apiKey: apiKey, model: "gpt-4o"}
}

// HealSelectors takes a base64-encoded screenshot of a Facebook page and returns
// new CSS selectors for the requested action. Returns a map of selector_name → css_selector.
func (h *SelectorHealer) HealSelectors(ctx context.Context, base64PNG, action, platform string) (map[string]string, error) {
	prompt := selectorPromptFor(action, platform)

	body := map[string]any{
		"model": h.model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "image_url",
						"image_url": map[string]any{
							"url":    "data:image/png;base64," + base64PNG,
							"detail": "high",
						},
					},
					{
						"type": "text",
						"text": prompt,
					},
				},
			},
		},
		"max_tokens":      512,
		"response_format": map[string]string{"type": "json_object"},
	}

	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai vision: %s", raw)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Choices) == 0 {
		return nil, fmt.Errorf("parse vision response: %w", err)
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	var selectors map[string]string
	if err := json.Unmarshal([]byte(content), &selectors); err != nil {
		return nil, fmt.Errorf("parse selectors json: %w", err)
	}
	return selectors, nil
}

func selectorPromptFor(action, platform string) string {
	switch action {
	case "scrape_posts":
		return `You are a web scraping expert analyzing a Facebook group page screenshot.
Identify CSS selectors for scraping posts. Return a JSON object with ONLY these keys:
- "post_container": selector for the repeating article/post wrapper element
- "post_text": selector for the main text content inside each post
- "author_name": selector for the author's display name
- "author_link": selector for the link to the author's profile (href)
- "post_link": selector for the permalink to the post
- "timestamp": selector for the time element

Rules:
1. Prefer [role="article"], [data-*], or aria attributes over random class names
2. Selectors must work relative to post_container (except post_container itself)
3. Return ONLY the JSON object, no explanation
4. If you cannot find a selector, use "" (empty string) as the value`

	case "post_comment":
		return `You are a web scraping expert analyzing a Facebook post page screenshot.
Identify CSS selectors for posting a comment. Return a JSON object with ONLY these keys:
- "comment_box": selector for the comment input area (contenteditable div or textarea)
- "submit_button": selector for the submit/send button (may be null if Enter key is used)
- "comment_form": selector for the wrapper form around the comment area

Rules:
1. Prefer [role="textbox"], [aria-label*="comment" i] over random class names
2. Return ONLY the JSON object, no explanation
3. If submit is via Enter key only, use "" for submit_button`

	default:
		return fmt.Sprintf(`Analyze this %s page screenshot and identify CSS selectors for action "%s".
Return a JSON object mapping selector names to CSS selector strings. Prefer aria/role/data attributes.`, platform, action)
	}
}

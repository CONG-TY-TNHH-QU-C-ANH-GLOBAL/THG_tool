package reel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// claudeClient calls the Anthropic Messages API (raw HTTP, mirroring the existing
// internal/ai OpenAI plumbing — no new SDK dependency, consistent with the codebase) to
// generate a real reel script. Selected when ANTHROPIC_API_KEY is set; model defaults to
// claude-sonnet-4-6 (the key provisioned for reel-script-engine-prod). The key is read
// from config/env — never hardcoded.
type claudeClient struct {
	apiKey string
	model  string
	http   *http.Client
}

func newClaudeClient(apiKey, model string) *claudeClient {
	if strings.TrimSpace(model) == "" {
		model = "claude-sonnet-4-6"
	}
	return &claudeClient{apiKey: strings.TrimSpace(apiKey), model: model, http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *claudeClient) available() bool { return c != nil && c.apiKey != "" }

// generate asks Claude for a structured reel script and parses the JSON it returns.
func (c *claudeClient) generate(ctx context.Context, in ScriptInput) (ScriptDraft, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      c.model,
		"max_tokens": 2000,
		"messages": []map[string]any{
			{"role": "user", "content": claudeReelPrompt(in)},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return ScriptDraft{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return ScriptDraft{}, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return ScriptDraft{}, fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, sliceStr(string(b), 200))
	}

	var out struct {
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ScriptDraft{}, err
	}
	// Guard the refusal stop reason before reading content (per Anthropic API).
	if out.StopReason == "refusal" {
		return ScriptDraft{}, fmt.Errorf("anthropic refused the request")
	}
	var text strings.Builder
	for _, b := range out.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	return parseScriptJSON(text.String())
}

// claudeReelPrompt builds a grounded Vietnamese prompt that demands strict JSON output.
func claudeReelPrompt(in ScriptInput) string {
	return fmt.Sprintf(`Bạn là copywriter video ngắn (reel) cho doanh nghiệp. Viết kịch bản reel ~%d giây.

THÔNG TIN DOANH NGHIỆP (chỉ dùng dữ kiện trong đây, KHÔNG bịa giá/website/cam kết):
%s%s

Ý TƯỞNG / PHONG CÁCH: %s
TỪ KHOÁ: %s

Chia 6-8 shot, mỗi shot 4-5 giây, mỗi shot KHÁC nhau (đừng lặp lại cùng một câu).
CHỈ trả về JSON hợp lệ đúng shape sau, không thêm chữ nào ngoài JSON:
{"dialogue":"lời thoại tổng tiếng Việt","caption":"caption kèm hashtag","shots":[{"scene":1,"kind":"talking_head|product|broll","prompt":"mô tả hình ảnh","dur_sec":5,"voiceover":"lời thoại tiếng Việt cho shot"}],"verify_flags":["số liệu/khẳng định cần con người xác minh"]}`,
		in.TargetDuration, strings.TrimSpace(in.BusinessBlock), marketingGuideBlock(in.MarketingGuide), in.BriefStyle, strings.Join(in.Keywords, ", "))
}

// parseScriptJSON extracts the first JSON object from text and decodes it into a ScriptDraft.
func parseScriptJSON(text string) (ScriptDraft, error) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ScriptDraft{}, fmt.Errorf("no JSON object in model output")
	}
	var draft ScriptDraft
	if err := json.Unmarshal([]byte(text[start:end+1]), &draft); err != nil {
		return ScriptDraft{}, fmt.Errorf("parse script JSON: %w", err)
	}
	if len(draft.Shots) == 0 {
		return ScriptDraft{}, fmt.Errorf("script JSON had no shots")
	}
	return draft, nil
}

// sliceStr safely truncates s to n runes for error messages.
func sliceStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

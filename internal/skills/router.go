package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrNeedsMoreContext is returned when GPT-4o requests clarification.
type ErrNeedsMoreContext struct {
	Message string // clarification question to show the user
}

func (e ErrNeedsMoreContext) Error() string {
	return "needs more context: " + e.Message
}

// RouteResult holds the resolved skill name and validated params.
type RouteResult struct {
	SkillName string
	Params    map[string]any
}

// SkillRouter uses GPT-4o function-calling to map free-text input to a skill.
type SkillRouter struct {
	apiKey   string
	model    string
	registry *Registry
	client   *http.Client
}

func NewSkillRouter(apiKey, model string, reg *Registry) *SkillRouter {
	if model == "" {
		model = "gpt-4o"
	}
	return &SkillRouter{
		apiKey:   apiKey,
		model:    model,
		registry: reg,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Route maps natural language text (Vietnamese or English) to a registered skill.
// Returns ErrNeedsMoreContext if GPT-4o requires clarification before routing.
func (r *SkillRouter) Route(ctx context.Context, text string) (RouteResult, error) {
	tools := r.buildTools()

	reqBody := map[string]any{
		"model": r.model,
		"messages": []map[string]any{
			{
				"role": "system",
				"content": `Bạn là dispatcher của hệ thống tự động Facebook.
Ánh xạ yêu cầu của người dùng sang đúng một skill. Tất cả tham số phải lấy từ tin nhắn của người dùng.
Nếu thiếu tham số bắt buộc hoặc ý định không rõ, hỏi lại bằng tiếng Việt.
Không bao giờ tự bịa giá trị tham số.`,
			},
			{"role": "user", "content": text},
		},
		"tools":       tools,
		"tool_choice": "auto",
	}

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return RouteResult{}, fmt.Errorf("skill router: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return RouteResult{}, fmt.Errorf("skill router: openai request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return RouteResult{}, fmt.Errorf("skill router: openai %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return RouteResult{}, fmt.Errorf("skill router: parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return RouteResult{}, errors.New("skill router: empty response from OpenAI")
	}

	msg := result.Choices[0].Message

	// No tool call → model wants clarification
	if len(msg.ToolCalls) == 0 {
		clarification := msg.Content
		if clarification == "" {
			clarification = "Bạn muốn làm gì? Vui lòng mô tả rõ hơn."
		}
		return RouteResult{}, ErrNeedsMoreContext{Message: clarification}
	}

	tc := msg.ToolCalls[0]
	var params map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
		return RouteResult{}, fmt.Errorf("skill router: parse params: %w", err)
	}
	if r.registry.Get(tc.Function.Name) == nil {
		return RouteResult{}, errors.New("skill router: unknown skill: " + tc.Function.Name)
	}

	return RouteResult{SkillName: tc.Function.Name, Params: params}, nil
}

func (r *SkillRouter) buildTools() []map[string]any {
	skills := r.registry.All()
	tools := make([]map[string]any, 0, len(skills))
	for _, s := range skills {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        s.Name(),
				"description": s.Description(),
				"parameters": map[string]any{
					"type":       "object",
					"properties": s.ParamSchema(),
				},
			},
		})
	}
	return tools
}

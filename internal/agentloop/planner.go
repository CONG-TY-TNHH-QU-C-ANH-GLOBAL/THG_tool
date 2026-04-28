package agentloop

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

// Planner is Phase 1 of the agent loop.
// It classifies the task domain, defines intent, and hypothesizes a root cause.
//
// Invariants:
//   - NEVER proposes patches (that is the Architect's job).
//   - NEVER hallucinates file names — it only sets domain + intent.
//   - Returns confidence < 0.5 when the task is ambiguous → triggers HUMAN_REQUIRED.
type Planner struct {
	apiKey string
	model  string
	client *http.Client
}

// NewPlanner creates a Planner backed by OpenAI.
func NewPlanner(apiKey, model string) *Planner {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &Planner{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

const plannerSystemPrompt = `You are the PLANNER phase of a self-healing production system.
Your ONLY job is to analyze a task and classify it. Do NOT propose patches or code changes.

Output STRICT JSON (no markdown, no explanation outside JSON):
{
  "domain": "browser | frontend | infra | job | unknown",
  "intent": "short description of what needs to be fixed",
  "root_cause": "hypothesis of the root cause",
  "confidence": 0.0-1.0
}

Domains:
- browser: Chrome/CDP/VNC stream, Facebook session, WebSocket
- frontend: Next.js/React, HTTP 4xx/5xx, build failures
- infra: Docker containers, nginx, CI/CD, database
- job: Scheduler, worker queue, job stuck/failed
- unknown: Cannot classify

Rules:
- If confidence < 0.5, use domain "unknown"
- Root cause must be a testable hypothesis (one sentence)
- Intent must be an action phrase ("fix", "restore", "restart")
- NEVER mention specific file names or function names`

// Plan classifies the task and returns a PlannerResult.
// Returns an error only on network/API failures — low confidence is expressed in the result.
func (p *Planner) Plan(ctx context.Context, task Task) (PlannerResult, error) {
	userMsg := fmt.Sprintf("Task: %s", task.Description)
	if task.Logs != "" {
		userMsg += fmt.Sprintf("\n\nLogs:\n%s", truncate(task.Logs, 2000))
	}

	body := map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": plannerSystemPrompt},
			{"role": "user", "content": userMsg},
		},
		"temperature":     0.1,
		"response_format": map[string]string{"type": "json_object"},
	}

	raw, err := p.call(ctx, body)
	if err != nil {
		return PlannerResult{}, err
	}

	var result PlannerResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return PlannerResult{}, fmt.Errorf("planner: parse response: %w — raw: %s", err, truncate(raw, 200))
	}
	if result.Domain == "" {
		result.Domain = DomainUnknown
	}
	return result, nil
}

func (p *Planner) call(ctx context.Context, body map[string]any) (string, error) {
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("planner: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("planner: OpenAI %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var oai struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &oai); err != nil {
		return "", fmt.Errorf("planner: decode: %w", err)
	}
	if len(oai.Choices) == 0 {
		return "", fmt.Errorf("planner: empty response")
	}
	return strings.TrimSpace(oai.Choices[0].Message.Content), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

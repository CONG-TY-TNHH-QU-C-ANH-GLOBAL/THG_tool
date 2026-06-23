package agentloop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Architect is Phase 2 of the agent loop.
// It reads the planner's intent, reads the relevant files, and produces a patch plan.
//
// Invariants:
//   - Every patch.File MUST exist on disk (checked by Executor, but Architect pre-validates too).
//   - Returns confidence < 0.6 → caller must escalate to HUMAN_REQUIRED.
//   - Risk "high" → caller must run risk gate before applying.
type Architect struct {
	apiKey string
	model  string
	client *http.Client
}

// NewArchitect creates an Architect backed by OpenAI.
func NewArchitect(apiKey, model string) *Architect {
	if model == "" {
		model = "gpt-4o"
	}
	return &Architect{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

const architectSystemPrompt = `You are the ARCHITECT phase of a self-healing production system.
Given a PLANNER intent and file contents, produce a minimal patch plan.

Output STRICT JSON (no markdown):
{
  "patches": [
    {
      "file": "relative/path/to/file.go",
      "action": "replace_block | insert_after | delete_block | prepend_import | append",
      "target": "exact string to find in the file (function name, line prefix, etc.)",
      "content": "full replacement or new content",
      "why": "one sentence explanation"
    }
  ],
  "confidence": 0.0-1.0,
  "risk": "low | medium | high",
  "rationale": "overall explanation in one paragraph"
}

Actions:
- replace_block: replace the block starting at "target" (finds the line containing target, replaces to matching closing brace)
- insert_after: insert "content" after the first line containing "target"
- delete_block: delete the block starting at "target"
- prepend_import: add import path "content" to the file's import block if not present
- append: append "content" to the end of the file

Rules:
- Only include files that ACTUALLY exist in the provided file list
- Each patch must have a unique, exact target string that exists in the file
- Confidence < 0.6 means you are unsure — output confidence honestly
- Risk "high" = touches auth/scheduler/runtime core; "medium" = config/handler; "low" = comment/log
- If the fix requires more context than provided, set confidence low and explain in rationale
- NEVER invent file paths not in the provided list
- Prefer the smallest possible change (surgical patch)`

// Rethink is appended to the architect prompt when a previous patch failed.
const architectRethinkPrompt = `
PREVIOUS ATTEMPT FAILED:
Reason: %s
Failed patches: %s

You MUST generate a DIFFERENT patch strategy. Do not repeat the same patches.
Analyze the failure reason and change your approach.`

// Design generates a patch plan from a planner result.
// If prevFailure is non-empty, the architect uses it to generate a different strategy.
func (a *Architect) Design(ctx context.Context, plan PlannerResult, availFiles []string, prevFailure string) (ArchitectResult, error) {
	// Read file contents for context (up to 5 most relevant files, truncated).
	fileContext := a.buildFileContext(availFiles, plan.Domain, plan.Intent)

	userMsg := fmt.Sprintf(
		"Domain: %s\nIntent: %s\nRoot cause: %s\n\nAvailable files:\n%s\n\nFile contents:\n%s",
		plan.Domain, plan.Intent, plan.RootCause,
		strings.Join(availFiles, "\n"),
		fileContext,
	)

	sysPrompt := architectSystemPrompt
	if prevFailure != "" {
		failedPatches := "(see previous trace)"
		sysPrompt += fmt.Sprintf(architectRethinkPrompt, prevFailure, failedPatches)
	}

	body := map[string]any{
		"model": a.model,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userMsg},
		},
		"temperature":     0.2,
		"response_format": map[string]string{"type": "json_object"},
	}

	raw, err := a.call(ctx, body)
	if err != nil {
		return ArchitectResult{}, err
	}

	var result ArchitectResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return ArchitectResult{}, fmt.Errorf("architect: parse: %w — raw: %s", err, truncate(raw, 200))
	}
	if result.Risk == "" {
		result.Risk = "medium"
	}
	return result, nil
}

// buildFileContext reads up to maxFilesForContext files and returns their content.
// Filters to files most likely relevant to the domain.
func (a *Architect) buildFileContext(files []string, domain Domain, intent string) string {
	const maxFilesForContext = 4
	const maxBytesPerFile = 3000

	relevant := rankFiles(files, domain, intent)
	if len(relevant) > maxFilesForContext {
		relevant = relevant[:maxFilesForContext]
	}

	var sb strings.Builder
	for _, f := range relevant {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > maxBytesPerFile {
			content = content[:maxBytesPerFile] + "\n// ... (truncated)"
		}
		sb.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", f, content))
	}
	return sb.String()
}

// rankFiles returns files sorted by relevance to the domain + intent keywords.
func rankFiles(files []string, domain Domain, intent string) []string {
	intentWords := strings.Fields(strings.ToLower(intent))

	domainKeywords := map[Domain][]string{
		DomainBrowser:  {"workspace", "vnc", "cdp", "browser", "chrome", "stream"},
		DomainFrontend: {"frontend", "page", "app", "layout", "component", "hook"},
		DomainInfra:    {"nginx", "docker", "deploy", "config", "main"},
		DomainJob:      {"job", "worker", "scheduler", "handler", "queue"},
	}

	keywords := append(domainKeywords[domain], intentWords...)

	type scored struct {
		file  string
		score int
	}
	var ranked []scored
	for _, f := range files {
		fl := strings.ToLower(f)
		s := 0
		for _, kw := range keywords {
			if strings.Contains(fl, kw) {
				s++
			}
		}
		ranked = append(ranked, scored{f, s})
	}

	// Simple insertion sort by descending score.
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].score > ranked[j-1].score; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	out := make([]string, len(ranked))
	for i, s := range ranked {
		out[i] = s.file
	}
	return out
}

func (a *Architect) call(ctx context.Context, body map[string]any) (string, error) {
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("architect: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("architect: OpenAI %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var oai struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &oai); err != nil {
		return "", fmt.Errorf("architect: decode: %w", err)
	}
	if len(oai.Choices) == 0 {
		return "", fmt.Errorf("architect: empty response")
	}
	return strings.TrimSpace(oai.Choices[0].Message.Content), nil
}

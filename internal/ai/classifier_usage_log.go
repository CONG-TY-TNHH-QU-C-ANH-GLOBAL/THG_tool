package ai

// Structured, redaction-safe usage logging for classifier LLM calls (Phase-1 cost
// observability). One JSON line per classifier decision; callers MUST pass only
// safe fields (ids, model, counts, booleans, hash prefixes) — never raw post text,
// prompt, completion, token, cookie, or auth header.

import (
	"encoding/json"
	"log"
	"strings"
)

// logClassifierUsage emits exactly one JSON line (parseable by ELK/Datadog/etc.).
func logClassifierUsage(fields map[string]any) {
	fields["event"] = "llm_usage"
	fields["task_type"] = "classifier"
	b, err := json.Marshal(fields)
	if err != nil {
		return // never let logging break classification
	}
	log.Printf("%s", b)
}

// classifyErrCode maps an error to a COARSE, content-free code for logs. It never
// logs the raw error string (which can carry an upstream response body).
func classifyErrCode(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "OpenAI HTTP 429"):
		return "http_429"
	case strings.Contains(s, "OpenAI HTTP 5"):
		return "http_5xx"
	case strings.Contains(s, "OpenAI HTTP 4"):
		return "http_4xx"
	case strings.Contains(s, "refused"):
		return "refused"
	case strings.Contains(s, "empty content"):
		return "empty_content"
	case strings.Contains(s, "context deadline"), strings.Contains(s, "context canceled"):
		return "context_cancelled"
	case strings.Contains(s, "request failed"):
		return "network"
	default:
		return "error"
	}
}

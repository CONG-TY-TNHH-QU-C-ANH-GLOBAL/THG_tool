package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// BusinessProfileInferrer extracts a structured business profile from a
// raw blob — either a website URL the user pasted (catalog, landing page)
// or a 1-line description. Output mirrors the 13 business_context fields
// the dashboard saves today, so the FE can pre-fill the form without any
// remapping.
//
// This is intentionally separate from Classifier (lead scoring). Profile
// inference reads marketing content; lead scoring reads social posts. The
// two prompts and the two failure modes have nothing in common.
type BusinessProfileInferrer struct {
	apiKey string
	model  string
	client *http.Client
}

func NewBusinessProfileInferrer(apiKey, model string) *BusinessProfileInferrer {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &BusinessProfileInferrer{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 45 * time.Second},
	}
}

func (i *BusinessProfileInferrer) Available() bool {
	return strings.TrimSpace(i.apiKey) != ""
}

// InferenceInput is what the FE submits. Exactly one of URL or Note is
// expected to be populated; if both are set, both are folded into the
// prompt so the LLM has a richer signal.
type InferenceInput struct {
	URL  string
	Note string
}

// InferredField is one extracted field paired with a confidence score in
// [0,1]. Confidence reflects how strongly the LLM thinks the value is
// grounded in the source — useful for the UI to surface a "review this"
// badge on weak fields.
type InferredField struct {
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

// InferenceResult is the wire shape returned to the FE. The 13 fields
// mirror updateBusinessContext exactly so the FE can map field-for-field.
type InferenceResult struct {
	BusinessProfile  InferredField `json:"business_profile"`
	BusinessName     InferredField `json:"business_name"`
	BusinessIndustry InferredField `json:"business_industry"`
	Services         InferredField `json:"services"`
	TargetCustomers  InferredField `json:"target_customers"`
	TargetAuthorRole InferredField `json:"target_author_role"`
	TargetSignals    InferredField `json:"target_signals"`
	NegativeSignals  InferredField `json:"negative_signals"`
	BusinessLocation InferredField `json:"business_location"`
	Markets          InferredField `json:"markets"`
	BusinessUSP      InferredField `json:"business_usp"`
	Tone             InferredField `json:"tone"`
	ApprovalPolicy   InferredField `json:"approval_policy"`
	RejectRules      InferredField `json:"reject_rules"`
	SourceSummary    string        `json:"source_summary"`
	SourceURL        string        `json:"source_url,omitempty"`
}

// Infer is the single entry point. It fetches the URL (if any), strips
// HTML to plain text, then calls the LLM with a strict JSON schema.
func (i *BusinessProfileInferrer) Infer(ctx context.Context, in InferenceInput) (*InferenceResult, error) {
	if !i.Available() {
		return nil, fmt.Errorf("AI inference not configured")
	}
	in.URL = strings.TrimSpace(in.URL)
	in.Note = strings.TrimSpace(in.Note)
	if in.URL == "" && in.Note == "" {
		return nil, fmt.Errorf("either url or note is required")
	}

	source := strings.Builder{}
	if in.Note != "" {
		source.WriteString("User-provided description:\n")
		source.WriteString(in.Note)
		source.WriteString("\n\n")
	}
	if in.URL != "" {
		page, err := i.fetchAndStrip(ctx, in.URL)
		if err != nil {
			// Fetching the page is best-effort. If it fails (network,
			// rate-limit, JS-only SPA) but we have a user note, still
			// proceed with note-only inference instead of erroring.
			if in.Note == "" {
				return nil, fmt.Errorf("fetch source URL: %w", err)
			}
			source.WriteString("(Note: could not fetch source URL: " + err.Error() + ")\n")
		} else {
			source.WriteString("Website content extracted from ")
			source.WriteString(in.URL)
			source.WriteString(":\n")
			source.WriteString(page)
		}
	}

	raw, err := i.callOpenAI(ctx, profileInferSystemPrompt(), source.String())
	if err != nil {
		return nil, err
	}
	result, err := parseInferenceResult(raw)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}
	result.SourceURL = in.URL
	return result, nil
}

// fetchAndStrip GETs the URL, caps the body at 1 MB, and returns plain
// text. Anything more elaborate (JS rendering, sitemap walk) belongs in
// a real ingestion pipeline, not this hot-path endpoint.
func (i *BusinessProfileInferrer) fetchAndStrip(ctx context.Context, raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("source_url must be an http(s) URL")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; THGProfileInferrer/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := i.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, raw)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB cap
	if err != nil {
		return "", err
	}
	text := stripHTML(string(body))
	// Cap the text we send to the LLM — Inter-content above ~16k chars
	// rarely adds signal and just burns tokens.
	if len(text) > 16000 {
		text = text[:16000]
	}
	return text, nil
}

var (
	reScript = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	reTag    = regexp.MustCompile(`<[^>]+>`)
	reWS     = regexp.MustCompile(`\s+`)
)

func stripHTML(s string) string {
	s = reScript.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = reWS.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func profileInferSystemPrompt() string {
	return `You analyse a website (or a short user description) and extract a structured business profile.

Return STRICT JSON matching exactly this schema. Every field has two keys:
"value" (string, may be empty if nothing relevant was found) and
"confidence" (number in [0,1] reflecting how strongly the value is
grounded in the source).

{
  "business_profile":   { "value": "1-3 sentence elevator pitch", "confidence": 0.0 },
  "business_name":      { "value": "brand or organisation name",   "confidence": 0.0 },
  "business_industry":  { "value": "industry / business model",    "confidence": 0.0 },
  "services":           { "value": "comma-separated services or product lines", "confidence": 0.0 },
  "target_customers":   { "value": "audience persona, pain points, segment", "confidence": 0.0 },
  "target_author_role": { "value": "one of: customers | suppliers | partners | candidates | providers", "confidence": 0.0 },
  "target_signals":     { "value": "comma-separated keywords / phrases an ideal lead would write", "confidence": 0.0 },
  "negative_signals":   { "value": "comma-separated keywords that suggest a NON-target (eg competitors)", "confidence": 0.0 },
  "business_location":  { "value": "primary HQ / city / country",  "confidence": 0.0 },
  "markets":            { "value": "comma-separated markets served (geographies)", "confidence": 0.0 },
  "business_usp":       { "value": "differentiator vs competitors","confidence": 0.0 },
  "tone":               { "value": "brand voice for outbound messages: short adjectives",  "confidence": 0.0 },
  "approval_policy":    { "value": "rules for what content needs admin approval", "confidence": 0.3 },
  "reject_rules":       { "value": "patterns to auto-reject in outbound", "confidence": 0.3 },
  "source_summary":     "1-sentence summary of what you read (plain string, no confidence)"
}

Rules:
- Output VALID JSON. No prose before or after.
- Confidence 0.0-0.3 = guessed / weak. 0.4-0.7 = inferred from context. 0.8-1.0 = directly stated.
- If a field is not derivable, set value="" and confidence=0.
- target_author_role defaults to "customers" when in doubt.
- Write field values in the same language as the source content (Vietnamese if source is Vietnamese, English otherwise).
- approval_policy / reject_rules are governance fields; confidence should stay low unless the source explicitly states a policy.`
}

func parseInferenceResult(raw string) (*InferenceResult, error) {
	// Some models wrap output in markdown fences; strip them.
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var result InferenceResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	// Defensive: clamp confidences to [0,1] in case the model misbehaved.
	clampField := func(f *InferredField) {
		if f.Confidence < 0 {
			f.Confidence = 0
		} else if f.Confidence > 1 {
			f.Confidence = 1
		}
		f.Value = strings.TrimSpace(f.Value)
	}
	clampField(&result.BusinessProfile)
	clampField(&result.BusinessName)
	clampField(&result.BusinessIndustry)
	clampField(&result.Services)
	clampField(&result.TargetCustomers)
	clampField(&result.TargetAuthorRole)
	clampField(&result.TargetSignals)
	clampField(&result.NegativeSignals)
	clampField(&result.BusinessLocation)
	clampField(&result.Markets)
	clampField(&result.BusinessUSP)
	clampField(&result.Tone)
	clampField(&result.ApprovalPolicy)
	clampField(&result.RejectRules)
	return &result, nil
}

func (i *BusinessProfileInferrer) callOpenAI(ctx context.Context, sysPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model": i.model,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature":     0.2,
		"response_format": map[string]string{"type": "json_object"},
	}
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+i.apiKey)
	resp, err := i.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}
	return result.Choices[0].Message.Content, nil
}

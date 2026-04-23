package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Classifier classifies scraped content using OpenAI.
type Classifier struct {
	apiKey string
	model  string
	db     *store.Store
	client *http.Client
}

// NewClassifier creates a new AI classifier using OpenAI.
func NewClassifier(apiKey, model string, db *store.Store) *Classifier {
	if model == "" {
		model = "gpt-4o-mini"
	}
	log.Printf("[AI Classifier] Initialized (model: %s)", model)
	return &Classifier{
		apiKey: apiKey,
		model:  model,
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Available returns true if the classifier has a valid API key.
func (c *Classifier) Available() bool {
	return c.apiKey != ""
}

// ClassifyBatch classifies a batch of posts and saves leads.
func (c *Classifier) ClassifyBatch(ctx context.Context, posts []models.Post) ([]models.Lead, error) {
	if !c.Available() {
		return nil, fmt.Errorf("OpenAI API key not configured")
	}

	sysPrompt := c.buildDynamicSystemPrompt()

	// Use business_industry as the niche label; fall back to active_niche for legacy compat
	activeNiche := ""
	if n, err := c.db.GetContext("business_industry"); err == nil && n != "" {
		activeNiche = n
	} else if n, err := c.db.GetContext("active_niche"); err == nil && n != "" {
		activeNiche = n
	}

	result, err := c.callOpenAI(ctx, sysPrompt, buildClassificationPrompt(posts, activeNiche))
	if err != nil {
		return nil, fmt.Errorf("classify failed: %w", err)
	}

	// DEBUG: log raw AI response
	log.Printf("[AI] Raw classification response (first 500 chars): %.500s", result)

	leads, err := c.parseClassification(result, posts, activeNiche)
	if err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	// Save leads to database
	for i := range leads {
		if _, err := c.db.InsertLead(&leads[i]); err != nil {
			log.Printf("[AI] Save lead error: %v", err)
		}
	}

	log.Printf("[AI] Classified %d posts → %d leads", len(posts), len(leads))
	return leads, nil
}

// buildDynamicSystemPrompt creates a classifier prompt from the stored BusinessProfile.
// Fully generic — works for any industry. No hardcoded niches.
func (c *Classifier) buildDynamicSystemPrompt() string {
	userCtx, _ := c.db.GetAllContext()
	profile := ProfileFromContext(userCtx)

	var sb strings.Builder
	sb.WriteString("You are a professional lead qualification AI.\n\n")

	// Inject the universal business profile block
	sb.WriteString(profile.ToPromptBlock())

	// If this is a recruitment business, also inject open jobs for scoring
	if strings.Contains(strings.ToLower(profile.Industry), "recruit") ||
		strings.EqualFold(userCtx["active_niche"], "tuyen_dung") {
		if jobs, err := c.db.GetActiveCareerJobs(); err == nil && len(jobs) > 0 {
			sb.WriteString("\nOPEN POSITIONS:\n")
			for _, j := range jobs {
				line := fmt.Sprintf("- %s", j.Title)
				if j.Location != "" {
					line += " (" + j.Location + ")"
				}
				if j.Requirements != "" {
					req := j.Requirements
					if len(req) > 150 {
						req = req[:150]
					}
					line += ": " + req
				}
				sb.WriteString(line + "\n")
			}
			sb.WriteString("Score candidates based on fit to these positions.\n")
		}
	}

	// Inject last search intent if available
	if intent := userCtx["last_search_intent"]; intent != "" {
		fmt.Fprintf(&sb, "\nUSER'S CURRENT GOAL: \"%s\"\n", intent)
	}

	sb.WriteString(`
SCORING SCALE:
- hot (0.8–1.0): Clear, urgent need matching our business. Act now.
- warm (0.5–0.7): Possible interest, worth reaching out.
- cold (0.2–0.4): Weak signal, low priority.
- rejected (0.0–0.1): Irrelevant, competitor, spam, or rule violation.

Respond ONLY in valid JSON format.`)

	return sb.String()
}

// callOpenAI calls OpenAI chat completions API.
func (c *Classifier) callOpenAI(ctx context.Context, sysPrompt, prompt string) (string, error) {
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature":     0.1,
		"response_format": map[string]string{"type": "json_object"},
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
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

func (c *Classifier) parseClassification(raw string, posts []models.Post, activeNiche string) ([]models.Lead, error) {
	var parsed struct {
		Results []struct {
			Index        int    `json:"index"`
			Role         string `json:"role"`
			AuthorRole   string `json:"author_role"`
			ServiceMatch string `json:"service_match"`
			Category     string `json:"category"`
			Score        string `json:"score"`
			PainPoint    string `json:"pain_point"`
			Reasoning    string `json:"reasoning"`
			Summary      string `json:"summary"`
			Analysis     string `json:"analysis"`
		} `json:"results"`
	}

	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse AI response: %w", err)
	}

	now := time.Now()
	var leads []models.Lead
	for _, r := range parsed.Results {
		if r.Index < 0 || r.Index >= len(posts) {
			continue
		}
		post := posts[r.Index]

		role := coalesce(r.AuthorRole, r.Role, "unknown")
		service := coalesce(r.ServiceMatch, r.Category, "None")
		reasoning := coalesce(r.Reasoning, r.Summary, r.Analysis, "")
		score := r.Score
		if score == "" {
			score = "cold"
		}

		if strings.EqualFold(score, "rejected") {
			continue
		}

		leads = append(leads, models.Lead{
			SourceType:   "post",
			SourceID:     post.ID,
			SourceURL:    post.URL,
			Platform:     post.Platform,
			Author:       post.Author,
			AuthorURL:    post.AuthorURL,
			Content:      post.Content,
			Score:        models.LeadScore(score),
			ServiceMatch: service,
			AuthorRole:   role,
			PainPoint:    r.PainPoint,
			AIReasoning:  reasoning,
			Niche:        activeNiche,
			ClassifiedAt: now,
		})
	}

	return leads, nil
}

func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func buildClassificationPrompt(posts []models.Post, _ string) string {
	var sb strings.Builder
	sb.WriteString("Classify the following social media posts based on the business profile in your system prompt. For each post:\n")
	sb.WriteString("- author_role: buyer/candidate/partner/seller/unknown (based on context)\n")
	sb.WriteString("- service_match: what specific product/service/position they need, or None\n")
	sb.WriteString("- score: hot, warm, cold, or rejected\n")
	sb.WriteString("- pain_point: their specific need or problem in one sentence\n")
	sb.WriteString("- reasoning: why you scored it this way\n\n")
	sb.WriteString(`Return JSON: {"results": [{"index": 0, "author_role": "...", "service_match": "...", "score": "...", "pain_point": "...", "reasoning": "..."}]}` + "\n\n")

	for i, p := range posts {
		content := p.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		fmt.Fprintf(&sb, "--- Post %d ---\nGroup: %s\nAuthor: %s\nContent: %s\n\n", i, p.GroupName, p.Author, content)
	}

	return sb.String()
}

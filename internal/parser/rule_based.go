package parser

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/thg/scraper/internal/jobs"
)

// intentKeywords maps intent names to Vietnamese/English trigger words.
var intentKeywords = map[string][]string{
	"facebook_crawl": {"cào", "scrape", "group", "nhóm", "crawl", "thu thập"},
	"lead_gen":       {"lead", "khách hàng", "tìm khách", "lead gen", "khách tiềm năng"},
	"visa_research":  {"visa", "du lịch", "hộ chiếu", "passport", "travel"},
	"web_crawl":      {"web", "website", "trang web", "url", "link"},
}

// RuleBasedParser detects intent and constructs a Task using keyword matching.
// It requires at least one Facebook group URL in the text for facebook_crawl intent.
type RuleBasedParser struct{}

func NewRuleBasedParser() *RuleBasedParser { return &RuleBasedParser{} }

func (p *RuleBasedParser) Parse(_ context.Context, text string) (*jobs.Task, error) {
	lower := strings.ToLower(text)
	intent := detectIntent(lower)
	if intent == "" {
		intent = "facebook_crawl"
	}

	keywords := extractKeywords(lower)
	urls := extractURLs(text)

	sources := make([]jobs.Source, 0, len(urls))
	for _, u := range urls {
		sources = append(sources, jobs.Source{
			Type: sourceTypeFor(intent),
			URL:  u,
		})
	}

	taskID := computeTaskID(intent, keywords)

	return &jobs.Task{
		SchemaVersion: "1",
		TaskID:        taskID,
		Intent:        intent,
		Keywords:      keywords,
		CrawlPlan: jobs.CrawlPlan{
			Sources:   sources,
			MaxItems:  100,
			BatchSize: 20,
		},
		Filters: jobs.Filters{
			Keywords:         keywords,
			MinContentLength: 20,
			KeywordMinScore:  0.1,
		},
		ScoringConfig: jobs.ScoringConfig{
			HotThreshold:  70,
			WarmThreshold: 40,
			Weights: jobs.ScoringWeights{
				KeywordRelevance: 0.40,
				Engagement:       0.30,
				ContentQuality:   0.30,
			},
		},
		RetryPolicy:         jobs.RetryPolicy{MaxAttempts: 3, BackoffMs: 1000},
		ExecutionMode:       "async",
		OutputSchema:        "facebook_crawl_v1",
		OutputSchemaVersion: "1",
	}, nil
}

func detectIntent(lower string) string {
	best, bestScore := "", 0
	for intent, triggers := range intentKeywords {
		score := 0
		for _, t := range triggers {
			if strings.Contains(lower, t) {
				score++
			}
		}
		if score > bestScore {
			bestScore, best = score, intent
		}
	}
	return best
}

func extractKeywords(lower string) []string {
	stopWords := map[string]bool{
		"cào": true, "scrape": true, "crawl": true, "nhóm": true,
		"group": true, "và": true, "the": true, "a": true, "an": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
	}
	tokens := strings.Fields(lower)
	seen := map[string]bool{}
	var kw []string
	for _, t := range tokens {
		t = strings.Trim(t, ".,!?;:")
		if len(t) < 3 || stopWords[t] || strings.HasPrefix(t, "http") {
			continue
		}
		if !seen[t] {
			seen[t] = true
			kw = append(kw, t)
		}
	}
	return kw
}

func extractURLs(text string) []string {
	var urls []string
	for _, tok := range strings.Fields(text) {
		if strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://") {
			urls = append(urls, strings.TrimRight(tok, ".,!?;:"))
		}
	}
	return urls
}

func sourceTypeFor(intent string) string {
	switch intent {
	case "facebook_crawl", "lead_gen":
		return "facebook_group"
	case "visa_research", "web_crawl":
		return "web_url"
	default:
		return "web_url"
	}
}

// computeTaskID produces a deterministic 16-char hex ID.
// Same intent + keywords + UTC day → same ID (prevents duplicate jobs within a day).
func computeTaskID(intent string, keywords []string) string {
	sorted := make([]string, len(keywords))
	copy(sorted, keywords)
	sort.Strings(sorted)

	day := time.Now().UTC().Format("2006-01-02")
	raw := intent + strings.Join(sorted, " ") + day
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum[:8])
}

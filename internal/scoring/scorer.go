package scoring

import (
	"fmt"
	"strings"
	"unicode"
)

// Config controls scoring thresholds and dimension weights.
// All weights should sum to 1.0.
type Config struct {
	HotThreshold  float64 // score >= this → "hot"   (default 70)
	WarmThreshold float64 // score >= this → "warm"  (default 40)
	Weights       Weights
}

type Weights struct {
	KeywordRelevance float64 // default 0.40
	Engagement       float64 // default 0.30
	ContentQuality   float64 // default 0.30
}

func DefaultConfig() Config {
	return Config{
		HotThreshold:  70,
		WarmThreshold: 40,
		Weights: Weights{
			KeywordRelevance: 0.40,
			Engagement:       0.30,
			ContentQuality:   0.30,
		},
	}
}

// Result is the output of a single Score call.
type Result struct {
	Score    float64  // 0–100
	Category string   // "hot" | "warm" | "cold"
	Signals  []string // human-readable signals that fired
}

// Scorer computes lead scores inline. Stateless and concurrency-safe.
type Scorer struct{ cfg Config }

func New(cfg Config) *Scorer { return &Scorer{cfg: cfg} }

// Score computes a 0–100 lead score from content signals.
// Called inside the crawl loop — must be O(n) where n = len(content).
func (s *Scorer) Score(content string, keywords []string, reactions, comments int, authorURL string) Result {
	var signals []string

	kwScore := keywordRelevance(content, keywords)
	if kwScore >= 0.6 {
		signals = append(signals, fmt.Sprintf("keyword_hit:%.0f%%", kwScore*100))
	}

	engScore := engagementScore(reactions, comments)
	if engScore >= 0.5 {
		signals = append(signals, fmt.Sprintf("engagement:%d", reactions+comments))
	}

	qualScore := contentQuality(content, authorURL)
	if qualScore >= 0.5 {
		signals = append(signals, "content_quality")
	}

	score := (kwScore*s.cfg.Weights.KeywordRelevance +
		engScore*s.cfg.Weights.Engagement +
		qualScore*s.cfg.Weights.ContentQuality) * 100

	category := "cold"
	switch {
	case score >= s.cfg.HotThreshold:
		category = "hot"
		signals = append(signals, "hot_lead")
	case score >= s.cfg.WarmThreshold:
		category = "warm"
	}

	return Result{Score: score, Category: category, Signals: signals}
}

// ── dimension functions ───────────────────────────────────────────────────────

func keywordRelevance(content string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0.5
	}
	lower := strings.ToLower(content)
	matched := 0
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			matched++
		}
	}
	return float64(matched) / float64(len(keywords))
}

func engagementScore(reactions, comments int) float64 {
	total := reactions + comments*2
	if total >= 50 {
		return 1.0
	}
	return float64(total) / 50.0
}

func contentQuality(content, authorURL string) float64 {
	var score float64
	runes := []rune(content)

	if len(runes) >= 50 {
		score += 0.3
	}
	if len(runes) >= 200 {
		score += 0.2
	}
	if authorURL != "" {
		score += 0.3
	}

	// penalise ALL-CAPS spam
	var letters, upper int
	for _, r := range runes {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters == 0 || float64(upper)/float64(letters) < 0.5 {
		score += 0.2
	}

	return score
}

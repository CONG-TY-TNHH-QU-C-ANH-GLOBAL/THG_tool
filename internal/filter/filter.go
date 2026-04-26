package filter

import (
	"strings"
	"time"
	"unicode"
)

// Config holds per-job filter parameters derived from jobs.Filters.
type Config struct {
	Keywords         []string
	ExcludePhrases   []string
	MinContentLength int
	MinReactions     int
	KeywordMinScore  float64
	TimeRangeStart   time.Time
	TimeRangeEnd     time.Time
}

// Result is the output of a single Evaluate call.
type Result struct {
	Pass    bool
	Score   float64
	Signals []string
}

// Engine runs a 4-stage inline filter pipeline.
// All stages run in order; the first FAIL short-circuits.
type Engine struct{}

func New() *Engine { return &Engine{} }

// Evaluate runs all filter stages against a single item.
// Called inside the crawl loop — never accumulate items before filtering.
func (e *Engine) Evaluate(
	content, authorProfileURL string,
	reactions int,
	timestamp time.Time,
	cfg Config,
) Result {
	var signals []string

	// Stage 1: Minimum content length
	if len([]rune(content)) < cfg.MinContentLength {
		return Result{Pass: false, Signals: []string{"content_too_short"}}
	}

	// Stage 2: Spam detection — exclude phrases + all-caps ratio
	lower := strings.ToLower(content)
	for _, phrase := range cfg.ExcludePhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return Result{Pass: false, Signals: []string{"excluded_phrase"}}
		}
	}
	if capsRatio(content) > 0.7 {
		return Result{Pass: false, Signals: []string{"all_caps_spam"}}
	}

	// Stage 3: Keyword relevance score
	score := keywordScore(lower, cfg.Keywords)
	if score < cfg.KeywordMinScore {
		return Result{Pass: false, Score: score, Signals: []string{"keyword_below_threshold"}}
	}
	signals = append(signals, "keyword_match")

	// Stage 4: Engagement gate (skip if all zeros in config)
	if cfg.MinReactions > 0 && reactions < cfg.MinReactions {
		return Result{Pass: false, Score: score, Signals: []string{"engagement_below_min"}}
	}

	// Stage 5: Quality — author profile required
	if authorProfileURL == "" {
		return Result{Pass: false, Score: score, Signals: []string{"missing_author_profile"}}
	}

	// Stage 6: Time range (skip if zero values)
	if !cfg.TimeRangeStart.IsZero() && timestamp.Before(cfg.TimeRangeStart) {
		return Result{Pass: false, Score: score, Signals: []string{"before_time_range"}}
	}
	if !cfg.TimeRangeEnd.IsZero() && timestamp.After(cfg.TimeRangeEnd) {
		return Result{Pass: false, Score: score, Signals: []string{"after_time_range"}}
	}

	return Result{Pass: true, Score: score, Signals: signals}
}

func keywordScore(lower string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 1.0
	}
	matched := 0
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			matched++
		}
	}
	return float64(matched) / float64(len(keywords))
}

func capsRatio(s string) float64 {
	var letters, upper int
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters == 0 {
		return 0
	}
	return float64(upper) / float64(letters)
}

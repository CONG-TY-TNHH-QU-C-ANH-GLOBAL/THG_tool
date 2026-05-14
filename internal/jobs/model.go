package jobs

import "time"

// Task is the structured document produced by the Parser layer.
// schema_version allows future evolution of this contract.
type Task struct {
	SchemaVersion       string        `json:"schema_version"`
	TaskID              string        `json:"task_id"`
	OrgID               int64         `json:"org_id"`
	AccountID           int64         `json:"account_id,omitempty"`
	// IntentID is the recurring crawl intent that produced this task (0 for
	// one-shot / user-prompted runs). Carried through so the ingest pipeline
	// can advance the per-intent cursor. See project_scheduled_intelligence.md.
	IntentID int64 `json:"intent_id,omitempty"`
	Intent   string `json:"intent"`
	Keywords            []string      `json:"keywords"`
	Entities            []string      `json:"entities,omitempty"`
	CrawlPlan           CrawlPlan     `json:"crawl_plan"`
	Filters             Filters       `json:"filters"`
	ScoringConfig       ScoringConfig `json:"scoring_config"`
	RetryPolicy         RetryPolicy   `json:"retry_policy"`
	ExecutionMode       string        `json:"execution_mode,omitempty"` // sync | async
	OutputSchema        string        `json:"output_schema"`
	OutputSchemaVersion string        `json:"output_schema_version,omitempty"` // e.g. "1"
	// Extras carries planner hints (e.g. market_signal_gate from Brain)
	// that downstream stages need but are not part of the strict crawl schema.
	Extras map[string]any `json:"extras,omitempty"`
}

// ScoringConfig controls lead scoring thresholds and dimension weights.
type ScoringConfig struct {
	HotThreshold  float64        `json:"hot_threshold"`  // default 70
	WarmThreshold float64        `json:"warm_threshold"` // default 40
	Weights       ScoringWeights `json:"weights"`
}

// ScoringWeights must sum to 1.0.
type ScoringWeights struct {
	KeywordRelevance float64 `json:"keyword_relevance"` // default 0.40
	Engagement       float64 `json:"engagement"`        // default 0.30
	ContentQuality   float64 `json:"content_quality"`   // default 0.30
}

// RetryPolicy controls job retry behaviour.
type RetryPolicy struct {
	MaxAttempts int `json:"max_attempts"` // default 3
	BackoffMs   int `json:"backoff_ms"`   // default 1000
}

// CrawlPlan describes what to crawl and how many items to collect.
type CrawlPlan struct {
	Sources   []Source `json:"sources"`
	MaxItems  int      `json:"max_items"`
	BatchSize int      `json:"batch_size"`
	// Recurring crawl cursor. When CursorLastPostID is non-empty, the crawler
	// SHOULD stop traversal upon encountering that post id (it has caught up
	// to the previous frontier). CursorLastPostAt / SinceRunAt are freshness
	// signals; post_id is the primary dedup key per the design mandate in
	// project_scheduled_intelligence.md (cursor is NEVER timestamp-only).
	CursorLastPostID string    `json:"cursor_last_post_id,omitempty"`
	CursorLastPostAt time.Time `json:"cursor_last_post_at,omitempty"`
	SinceRunAt       time.Time `json:"since_run_at,omitempty"`
}

// Source is a single crawl target.
type Source struct {
	Type  string `json:"type"` // facebook_group | facebook_post | facebook_profile | web_url
	URL   string `json:"url"`
	Label string `json:"label,omitempty"`
}

// Filters are applied per-item DURING crawling, never post-collection.
type Filters struct {
	Keywords         []string `json:"keywords"`
	ExcludePhrases   []string `json:"exclude_phrases"`
	MinContentLength int      `json:"min_content_length"`
	MinReactions     int      `json:"min_reactions"`
	KeywordMinScore  float64  `json:"keyword_min_score"`
}

// Job is a row in the jobs table.
type Job struct {
	ID          int64      `json:"id"`
	TaskID      string     `json:"task_id"`
	Intent      string     `json:"intent"`
	Payload     string     `json:"payload"`
	Status      string     `json:"status"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"max_attempts"`
	Error       string     `json:"error,omitempty"`
	ClaimedBy   string     `json:"claimed_by,omitempty"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
	Progress    int        `json:"progress"` // 0–100, updated by handler
	Result      string     `json:"result,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

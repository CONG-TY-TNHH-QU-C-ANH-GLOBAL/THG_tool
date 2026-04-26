package output

import "time"

// Record is one filtered and scored item from the crawl loop.
// Matches the final system output contract: records[].data + lead_score + category + signals.
type Record struct {
	// Raw crawl data
	ID               string    `json:"id"`
	Content          string    `json:"content"`
	AuthorName       string    `json:"author_name"`
	AuthorProfileURL string    `json:"author_profile_url"`
	SourceURL        string    `json:"source_url"`
	Timestamp        time.Time `json:"timestamp"`
	Reactions        int       `json:"reactions"`
	Comments         int       `json:"comments"`
	Shares           int       `json:"shares"`

	// Scoring
	LeadScore float64  `json:"lead_score"`          // 0–100
	Category  string   `json:"category"`             // hot | warm | cold
	Signals   []string `json:"signals"`              // combined filter + scoring signals

	// Legacy / backward compat
	FilterSignals []string `json:"filter_signals,omitempty"`
}

// Stats is the execution summary appended to every Dataset.
type Stats struct {
	TotalFetched  int `json:"total_fetched"`
	TotalFiltered int `json:"total_filtered"`
	TotalDeduped  int `json:"total_deduped"`
	TotalReturned int `json:"total_returned"`
}

// Dataset is the JSON stored in jobs.result on completion.
// Output schema version 1: records[] with inline lead scores.
type Dataset struct {
	Records  []Record `json:"records"`
	Stats    Stats    `json:"stats"`
	Insights []any    `json:"insights"`
}

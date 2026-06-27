package soak

import "time"

// Report is the operator-readable output of one soak run. It is the
// SINGLE source of truth for "is the retrieval substrate ready to
// trust?" — every measurement called out in the production-soak
// goal directive maps to one field here.
//
// Reports are JSON-serialisable for archival (operators commit a
// soak result to specs/RETRIEVAL_SOAK_<date>.json when they cut a
// release) and Markdown-printable for human review.
type Report struct {
	GeneratedAt   time.Time `json:"generated_at"`
	HarnessConfig HarnessConfig `json:"harness_config"`

	// Catalog summary: what got loaded into the test store.
	CatalogSize        int            `json:"catalog_size"`
	EmbeddingsGenerated int           `json:"embeddings_generated"`
	EmbeddingsPending  int            `json:"embeddings_pending"`
	EmbeddingsFailed   int            `json:"embeddings_failed"`
	AssetsByType       map[string]int `json:"assets_by_type"`

	// Per-prompt outcomes — the heart of the soak.
	PromptOutcomes []PromptOutcome `json:"prompt_outcomes"`

	// Aggregate retrieval quality (across all prompts).
	Quality QualityMetrics `json:"quality"`

	// Fallback behaviour observability (goal directive §1).
	FallbackBehaviour FallbackMetrics `json:"fallback_behaviour"`

	// Stale knowledge detection (§3).
	StaleDetection StaleMetrics `json:"stale_detection"`

	// Replay explainability — verifies every trace is well-formed.
	ReplayHealth ReplayHealth `json:"replay_health"`

	// Failure-mode scenarios A-F.
	FailureModes []FailureModeOutcome `json:"failure_modes"`

	// Operator trust score — composite that signals "ready or not".
	OperatorTrust TrustVerdict `json:"operator_trust"`

	// LoadProfile records what the soak observed under concurrent
	// load. Populated only by the "real-soak" variant that drives
	// the OpenAIEmbedder against the FakeOpenAI server.
	LoadProfile *LoadProfile `json:"load_profile,omitempty"`

	// CostTelemetry tracks token usage from the fake-OpenAI server
	// counters. Real-soak only.
	CostTelemetry *CostTelemetry `json:"cost_telemetry,omitempty"`

	// TenantIsolation: cross-org concurrent queries verified to
	// never leak. Real-soak only.
	TenantIsolation *TenantIsolation `json:"tenant_isolation,omitempty"`

	// Notes accumulated by the harness (warnings, observations).
	Notes []string `json:"notes,omitempty"`
}

// LoadProfile is the result of running concurrent retrievals.
type LoadProfile struct {
	Concurrency       int     `json:"concurrency"`
	TotalQueries      int     `json:"total_queries"`
	SuccessfulQueries int     `json:"successful_queries"`
	WallClockMs       int64   `json:"wall_clock_ms"`
	QPS               float64 `json:"queries_per_second"`
	P50LatencyMs      int64   `json:"p50_latency_ms"`
	P95LatencyMs      int64   `json:"p95_latency_ms"`
	P99LatencyMs      int64   `json:"p99_latency_ms"`
	MaxLatencyMs      int64   `json:"max_latency_ms"`
}

// CostTelemetry surfaces the token-usage observed during the soak.
// Production operators read this to size their OpenAI billing tier
// before scaling.
type CostTelemetry struct {
	EmbeddingRequests   int64   `json:"embedding_requests"`
	EmbeddingTokens     int64   `json:"embedding_tokens"`
	Failures429         int64   `json:"failures_429"`
	Failures5xx         int64   `json:"failures_5xx"`
	EstimatedCostUSD    float64 `json:"estimated_cost_usd"` // per text-embedding-3-small pricing
	AvgTokensPerRequest float64 `json:"avg_tokens_per_request"`
}

// TenantIsolation records the cross-org-concurrent-query result.
// Each query carries an explicit OrgID; the assertion is that no
// query saw an asset belonging to another org.
type TenantIsolation struct {
	OrgsExercised int `json:"orgs_exercised"`
	QueriesPerOrg int `json:"queries_per_org"`
	LeaksDetected int `json:"leaks_detected"`
}

// HarnessConfig records what variant of the run produced this Report.
// Two runs of the same harness with different config produce different
// numbers — the config makes them comparable.
type HarnessConfig struct {
	EmbedderModel        string `json:"embedder_model"`
	EmbeddingDimensions  int    `json:"embedding_dimensions"`
	SearcherDescription  string `json:"searcher_description"` // e.g. "rrf(hybrid, pgvector-mock)"
	TopK                 int    `json:"top_k"`
	MinConfidence        float64 `json:"min_confidence"`
}

// PromptOutcome captures everything the soak measured for ONE lead.
// This is the row a human operator inspects when they want to audit
// "what did the system retrieve for this lead, and was it right?"
type PromptOutcome struct {
	Prompt          string         `json:"prompt"`
	Language        string         `json:"language"`
	ExpectedIntent  []string       `json:"expected_intent"`
	RetrievedTitles []string       `json:"retrieved_titles"`
	TopScore        float64        `json:"top_score"`
	PrecisionAtK    float64        `json:"precision_at_k"`
	LatencyMs       int64          `json:"latency_ms"`
	SearcherImpl    string         `json:"searcher_impl"`
	FellBackTo      string         `json:"fell_back_to,omitempty"`
	ComplianceLeaks []string       `json:"compliance_leaks,omitempty"` // banned-claim titles found — MUST be empty
	HiddenLeaks     []string       `json:"hidden_leaks,omitempty"`     // hidden-state titles found — MUST be empty
	BelowMinScore   bool           `json:"below_min_score,omitempty"`
	TraceComplete   bool           `json:"trace_complete"`
	Verdict         string         `json:"verdict"` // "PASS" | "FAIL" | "DEGRADED"
}

// QualityMetrics aggregates precision across all prompts. The mean
// is the headline number; the distribution shows tail behaviour.
type QualityMetrics struct {
	MeanPrecisionAtK    float64 `json:"mean_precision_at_k"`
	MedianPrecisionAtK  float64 `json:"median_precision_at_k"`
	P10PrecisionAtK     float64 `json:"p10_precision_at_k"` // worst 10%
	PassedPrompts       int     `json:"passed_prompts"`
	FailedPrompts       int     `json:"failed_prompts"`
	DegradedPrompts     int     `json:"degraded_prompts"`
	AvgRetrievedCount   float64 `json:"avg_retrieved_count"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	P95LatencyMs        int64   `json:"p95_latency_ms"`
}

// FallbackMetrics measure when and why the pgvector → hybrid
// fallback fired during the soak. The goal directive (§1) requires
// fallback to be a non-event for healthy paths.
type FallbackMetrics struct {
	TotalQueries     int            `json:"total_queries"`
	FallbackInvocations int         `json:"fallback_invocations"`
	FallbackRate     float64        `json:"fallback_rate"`
	ByReason         map[string]int `json:"by_reason"`
}

// StaleMetrics reports on the catalog's freshness profile. Stale
// detection (§3) means the system can identify assets that have
// stopped surfacing, not that it auto-removes them.
type StaleMetrics struct {
	TotalAssets        int `json:"total_assets"`
	NeverRetrieved     int `json:"never_retrieved"`
	StalePast30d       int `json:"stale_past_30d"`
	FreshLast24h       int `json:"fresh_last_24h"`
}

// ReplayHealth verifies the OBSERVABILITY substrate itself. Every
// retrieval MUST produce a complete trace; missing fields would
// silently degrade the Operator Replay surface.
type ReplayHealth struct {
	TracesProduced       int     `json:"traces_produced"`
	TracesComplete       int     `json:"traces_complete"`
	CompletenessRate     float64 `json:"completeness_rate"`
	MissingSearcherImpl  int     `json:"missing_searcher_impl"`
	MissingSelected      int     `json:"missing_selected"`
	MissingScoreBreakdown int    `json:"missing_score_breakdown"`
}

// FailureModeOutcome records the result of injecting one production
// failure scenario. Maps to PR-4 §5 (A through F).
type FailureModeOutcome struct {
	ID          string `json:"id"`       // "A", "B", ...
	Name        string `json:"name"`     // human-readable
	Description string `json:"description"`
	Behaviour   string `json:"behaviour"` // observed system behaviour
	Verdict     string `json:"verdict"`  // PASS / FAIL
}

// TrustVerdict is the composite verdict. Operators read THIS first
// — the rest is supporting detail. Goal directive §2 (replay
// auditability) + §6 (cost telemetry) + §7 (quality benchmark) all
// contribute.
type TrustVerdict struct {
	Score            int      `json:"score"`     // 0-100
	Verdict          string   `json:"verdict"`   // "READY" | "DEGRADED" | "NOT_READY"
	BlockingIssues   []string `json:"blocking_issues,omitempty"`
	WarningIssues    []string `json:"warning_issues,omitempty"`
}

// ToMarkdown returns the report as operator-readable markdown.
// Designed for printing to test logs AND for pasting into a slack

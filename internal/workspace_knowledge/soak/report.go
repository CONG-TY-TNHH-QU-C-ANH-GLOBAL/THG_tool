package soak

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

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
// thread when the operator runs the harness against production.
func (r *Report) ToMarkdown() string {
	var b strings.Builder
	r.writeHeader(&b)
	r.writeTrust(&b)
	r.writeCatalog(&b)
	r.writeQuality(&b)
	r.writeFallback(&b)
	r.writeReplayHealth(&b)
	r.writePromptOutcomes(&b)
	r.writeCompliance(&b)
	r.writeFailureModes(&b)
	r.writeRealSoakSections(&b)
	r.writeNotes(&b)
	return b.String()
}

func (r *Report) writeHeader(b *strings.Builder) {
	fmt.Fprintf(b, "# Retrieval Substrate Soak Report\n\n")
	fmt.Fprintf(b, "**Generated:** %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(b, "**Searcher:** %s · **Embedder:** %s (%d dims) · **k=%d**\n\n",
		r.HarnessConfig.SearcherDescription, r.HarnessConfig.EmbedderModel,
		r.HarnessConfig.EmbeddingDimensions, r.HarnessConfig.TopK)
}

// writeTrust renders the headline operator-trust verdict first.
func (r *Report) writeTrust(b *strings.Builder) {
	fmt.Fprintf(b, "## Operator Trust: **%s** (score: %d/100)\n\n", r.OperatorTrust.Verdict, r.OperatorTrust.Score)
	if len(r.OperatorTrust.BlockingIssues) > 0 {
		fmt.Fprintf(b, "### Blocking issues\n")
		for _, issue := range r.OperatorTrust.BlockingIssues {
			fmt.Fprintf(b, "- 🛑 %s\n", issue)
		}
		fmt.Fprintln(b)
	}
	if len(r.OperatorTrust.WarningIssues) > 0 {
		fmt.Fprintf(b, "### Warnings\n")
		for _, w := range r.OperatorTrust.WarningIssues {
			fmt.Fprintf(b, "- ⚠️ %s\n", w)
		}
		fmt.Fprintln(b)
	}
}

func (r *Report) writeCatalog(b *strings.Builder) {
	fmt.Fprintf(b, "## Catalog\n")
	fmt.Fprintf(b, "- Total assets: %d\n", r.CatalogSize)
	fmt.Fprintf(b, "- Embeddings generated: %d / pending: %d / failed: %d\n",
		r.EmbeddingsGenerated, r.EmbeddingsPending, r.EmbeddingsFailed)
	fmt.Fprintf(b, "- By type:\n")
	for k, v := range r.AssetsByType {
		fmt.Fprintf(b, "  - %s: %d\n", k, v)
	}
	fmt.Fprintln(b)
}

func (r *Report) writeQuality(b *strings.Builder) {
	fmt.Fprintf(b, "## Retrieval Quality\n")
	fmt.Fprintf(b, "- Mean Precision@K: %.2f · Median: %.2f · P10: %.2f\n",
		r.Quality.MeanPrecisionAtK, r.Quality.MedianPrecisionAtK, r.Quality.P10PrecisionAtK)
	fmt.Fprintf(b, "- Prompts PASS: %d · FAIL: %d · DEGRADED: %d\n",
		r.Quality.PassedPrompts, r.Quality.FailedPrompts, r.Quality.DegradedPrompts)
	fmt.Fprintf(b, "- Avg retrieved per prompt: %.1f\n", r.Quality.AvgRetrievedCount)
	fmt.Fprintf(b, "- Latency avg: %.1fms · p95: %dms\n\n",
		r.Quality.AvgLatencyMs, r.Quality.P95LatencyMs)
}

func (r *Report) writeFallback(b *strings.Builder) {
	fmt.Fprintf(b, "## Fallback Behaviour\n")
	fmt.Fprintf(b, "- Fallback rate: %.1f%% (%d / %d)\n",
		r.FallbackBehaviour.FallbackRate*100,
		r.FallbackBehaviour.FallbackInvocations, r.FallbackBehaviour.TotalQueries)
	if len(r.FallbackBehaviour.ByReason) > 0 {
		fmt.Fprintf(b, "- By reason:\n")
		for reason, count := range r.FallbackBehaviour.ByReason {
			fmt.Fprintf(b, "  - %s: %d\n", reason, count)
		}
	}
	fmt.Fprintln(b)
}

func (r *Report) writeReplayHealth(b *strings.Builder) {
	fmt.Fprintf(b, "## Replay Health\n")
	fmt.Fprintf(b, "- Traces complete: %d / %d (%.1f%%)\n",
		r.ReplayHealth.TracesComplete, r.ReplayHealth.TracesProduced,
		r.ReplayHealth.CompletenessRate*100)
	if r.ReplayHealth.MissingSearcherImpl > 0 {
		fmt.Fprintf(b, "- ⚠️ Missing SearcherImpl: %d\n", r.ReplayHealth.MissingSearcherImpl)
	}
	if r.ReplayHealth.MissingSelected > 0 {
		fmt.Fprintf(b, "- ⚠️ Missing Selected: %d\n", r.ReplayHealth.MissingSelected)
	}
	fmt.Fprintln(b)
}

func (r *Report) writePromptOutcomes(b *strings.Builder) {
	fmt.Fprintf(b, "## Per-Prompt Outcomes\n\n")
	fmt.Fprintf(b, "| Lang | Verdict | Score | P@K | Lat ms | Prompt |\n")
	fmt.Fprintf(b, "|---|---|---|---|---|---|\n")
	for _, p := range r.PromptOutcomes {
		emoji := "✅"
		switch p.Verdict {
		case "FAIL":
			emoji = "❌"
		case "DEGRADED":
			emoji = "⚠️"
		}
		// Truncate prompt for table.
		shown := p.Prompt
		if len(shown) > 60 {
			shown = shown[:60] + "…"
		}
		fmt.Fprintf(b, "| %s | %s %s | %.2f | %.2f | %d | %s |\n",
			p.Language, emoji, p.Verdict, p.TopScore, p.PrecisionAtK, p.LatencyMs, shown)
	}
	fmt.Fprintln(b)
}

// writeCompliance is called out separately because it's the
// highest-severity safety signal.
func (r *Report) writeCompliance(b *strings.Builder) {
	complianceClean := true
	for _, p := range r.PromptOutcomes {
		if len(p.ComplianceLeaks) > 0 || len(p.HiddenLeaks) > 0 {
			complianceClean = false
		}
	}
	fmt.Fprintf(b, "## Compliance / Governance\n")
	if complianceClean {
		fmt.Fprintf(b, "✅ No banned claims surfaced. No hidden-state assets surfaced.\n\n")
	} else {
		fmt.Fprintf(b, "🛑 COMPLIANCE VIOLATIONS:\n")
		for _, p := range r.PromptOutcomes {
			for _, leak := range p.ComplianceLeaks {
				fmt.Fprintf(b, "- Prompt %q surfaced banned: %s\n", p.Prompt, leak)
			}
			for _, leak := range p.HiddenLeaks {
				fmt.Fprintf(b, "- Prompt %q surfaced hidden: %s\n", p.Prompt, leak)
			}
		}
		fmt.Fprintln(b)
	}
}

func (r *Report) writeFailureModes(b *strings.Builder) {
	fmt.Fprintf(b, "## Failure Mode Scenarios\n")
	for _, f := range r.FailureModes {
		emoji := "✅"
		if f.Verdict != "PASS" {
			emoji = "❌"
		}
		fmt.Fprintf(b, "- %s **%s** — %s: %s\n", emoji, f.ID, f.Name, f.Behaviour)
	}
	fmt.Fprintln(b)
}

// writeRealSoakSections renders the real-soak sections only when populated.
func (r *Report) writeRealSoakSections(b *strings.Builder) {
	if r.LoadProfile != nil {
		lp := r.LoadProfile
		fmt.Fprintf(b, "## Concurrent Load Profile\n")
		fmt.Fprintf(b, "- Concurrency: %d workers · %d total queries\n",
			lp.Concurrency, lp.TotalQueries)
		fmt.Fprintf(b, "- Successful: %d (%.1f%%)\n",
			lp.SuccessfulQueries,
			100.0*float64(lp.SuccessfulQueries)/float64(max(lp.TotalQueries, 1)))
		fmt.Fprintf(b, "- Wall clock: %dms · throughput: %.1f QPS\n",
			lp.WallClockMs, lp.QPS)
		fmt.Fprintf(b, "- Latency: p50=%dms · p95=%dms · p99=%dms · max=%dms\n\n",
			lp.P50LatencyMs, lp.P95LatencyMs, lp.P99LatencyMs, lp.MaxLatencyMs)
	}
	if r.CostTelemetry != nil {
		ct := r.CostTelemetry
		fmt.Fprintf(b, "## Cost Telemetry (real HTTP exercise)\n")
		fmt.Fprintf(b, "- Embedding requests: %d · tokens served: %d\n",
			ct.EmbeddingRequests, ct.EmbeddingTokens)
		fmt.Fprintf(b, "- Avg tokens / request: %.1f\n", ct.AvgTokensPerRequest)
		fmt.Fprintf(b, "- Failures: %d × 429 (rate limit) · %d × 5xx\n",
			ct.Failures429, ct.Failures5xx)
		fmt.Fprintf(b, "- Estimated cost: $%.6f (text-embedding-3-small @ $0.02/1M tokens)\n\n",
			ct.EstimatedCostUSD)
	}
	if r.TenantIsolation != nil {
		ti := r.TenantIsolation
		emoji := "✅"
		if ti.LeaksDetected > 0 {
			emoji = "🛑"
		}
		fmt.Fprintf(b, "## Tenant Isolation Under Concurrent Load\n")
		fmt.Fprintf(b, "%s %d orgs × %d queries/org · %d leaks detected\n\n",
			emoji, ti.OrgsExercised, ti.QueriesPerOrg, ti.LeaksDetected)
	}
}

func (r *Report) writeNotes(b *strings.Builder) {
	if len(r.Notes) > 0 {
		fmt.Fprintf(b, "## Notes\n")
		for _, n := range r.Notes {
			fmt.Fprintf(b, "- %s\n", n)
		}
	}
}

// ToJSON returns the report as pretty-printed JSON for archival.
func (r *Report) ToJSON() string {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(b)
}

// computeQualityAggregates calculates the headline metrics from a
// slice of PromptOutcomes. Pure function; safe to call multiple
// times.
func computeQualityAggregates(outcomes []PromptOutcome) QualityMetrics {
	if len(outcomes) == 0 {
		return QualityMetrics{}
	}
	q := QualityMetrics{}
	var precisions []float64
	var latencies []int64
	var totalRetrieved int

	for _, o := range outcomes {
		precisions = append(precisions, o.PrecisionAtK)
		latencies = append(latencies, o.LatencyMs)
		totalRetrieved += len(o.RetrievedTitles)
		switch o.Verdict {
		case "PASS":
			q.PassedPrompts++
		case "FAIL":
			q.FailedPrompts++
		case "DEGRADED":
			q.DegradedPrompts++
		}
	}

	sort.Float64s(precisions)
	q.MeanPrecisionAtK = mean(precisions)
	q.MedianPrecisionAtK = percentile(precisions, 50)
	q.P10PrecisionAtK = percentile(precisions, 10)
	q.AvgRetrievedCount = float64(totalRetrieved) / float64(len(outcomes))

	// Latency aggregates.
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	var latSum float64
	for _, l := range latencies {
		latSum += float64(l)
	}
	q.AvgLatencyMs = latSum / float64(len(latencies))
	q.P95LatencyMs = latencies[int(float64(len(latencies))*0.95)]
	return q
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted) * p) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

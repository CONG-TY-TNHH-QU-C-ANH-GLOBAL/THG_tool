package soak

import (
	"fmt"
	"strings"
)

// Markdown rendering of a soak Report — compliance/failure-mode/real-soak detail
// sections. Split from report.go; output behavior-locked (RETRIEVAL_SOAK_REPORT.md).

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

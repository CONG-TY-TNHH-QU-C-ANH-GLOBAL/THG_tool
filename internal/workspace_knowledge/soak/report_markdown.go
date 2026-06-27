package soak

import (
	"fmt"
	"strings"
	"time"
)

// Markdown rendering of a soak Report — summary sections. Split from report.go
// (data model). Output is behavior-locked: it drives RETRIEVAL_SOAK_REPORT.md.

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

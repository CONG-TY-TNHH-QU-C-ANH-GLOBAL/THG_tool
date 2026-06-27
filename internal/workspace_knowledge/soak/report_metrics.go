package soak

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Report serialisation (JSON) + the pure quality-aggregate computation. Split from
// report.go; pure functions, no behavior change.

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

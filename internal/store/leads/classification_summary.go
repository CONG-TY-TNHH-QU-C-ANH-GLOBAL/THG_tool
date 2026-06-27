// Domain: leads (see internal/store/DOMAINS.md)
package leads

import "strings"

// Pure in-memory folding of classification_log rows into a breakdown. Split out of
// classification_log.go so the SQL/scan stays there and these stay directly testable.

// accumulateClassificationRow folds one classification_log row into the running
// breakdown (kept/rejected totals, by-intent buckets, reason hit counts). Pure —
// no DB. Extracted from SummariseClassifications' scan loop; behavior unchanged.
func accumulateClassificationRow(out *ClassificationBreakdown, reasonHits map[string]int, decision, intent, reason string) {
	out.Total++
	switch decision {
	case ClassificationKept:
		out.Kept++
	case ClassificationRejected, ClassificationCold:
		out.Rejected++
		key := strings.TrimSpace(intent)
		if key == "" {
			key = "(no intent)"
		}
		out.ByIntent[key]++
		if r := strings.TrimSpace(reason); r != "" {
			reasonHits[r]++
		}
	}
}

// topReasons returns the n most-frequent reasons, highest count first (stable
// insertion sort, matching the former inline order). Pure — extracted from
// SummariseClassifications; behavior unchanged.
func topReasons(reasonHits map[string]int, n int) []ReasonCount {
	type kv struct {
		k string
		v int
	}
	arr := make([]kv, 0, len(reasonHits))
	for k, v := range reasonHits {
		arr = append(arr, kv{k, v})
	}
	for i := 1; i < len(arr); i++ {
		for j := i; j > 0 && arr[j].v > arr[j-1].v; j-- {
			arr[j-1], arr[j] = arr[j], arr[j-1]
		}
	}
	if len(arr) > n {
		arr = arr[:n]
	}
	out := make([]ReasonCount, len(arr))
	for i, kv := range arr {
		out[i] = ReasonCount{Reason: kv.k, Count: kv.v}
	}
	return out
}

package leads

import "testing"

// accumulateClassificationRow must tally kept/rejected totals, bucket rejected
// rows by intent (blank intent → "(no intent)"), and count non-blank reasons.
func TestAccumulateClassificationRow(t *testing.T) {
	out := &ClassificationBreakdown{ByIntent: map[string]int{}}
	hits := map[string]int{}

	accumulateClassificationRow(out, hits, ClassificationKept, "buyer", "looks good")
	accumulateClassificationRow(out, hits, ClassificationRejected, "buyer", "off topic")
	accumulateClassificationRow(out, hits, ClassificationCold, "", "off topic")
	accumulateClassificationRow(out, hits, ClassificationRejected, "spammer", "  ") // blank reason

	if out.Total != 4 || out.Kept != 1 || out.Rejected != 3 {
		t.Fatalf("totals: total=%d kept=%d rejected=%d, want 4/1/3", out.Total, out.Kept, out.Rejected)
	}
	if out.ByIntent["buyer"] != 1 || out.ByIntent["(no intent)"] != 1 || out.ByIntent["spammer"] != 1 {
		t.Fatalf("by-intent buckets wrong: %v", out.ByIntent)
	}
	if hits["off topic"] != 2 {
		t.Fatalf("reason 'off topic' should count 2, got %d", hits["off topic"])
	}
	if _, blankCounted := hits[""]; blankCounted {
		t.Fatalf("blank reason must not be counted: %v", hits)
	}
}

// topReasons must return highest-count-first and cap at n.
func TestTopReasons(t *testing.T) {
	hits := map[string]int{"a": 5, "b": 9, "c": 1, "d": 7}
	got := topReasons(hits, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 capped reasons, got %d (%v)", len(got), got)
	}
	if got[0].Reason != "b" || got[0].Count != 9 || got[1].Reason != "d" || got[1].Count != 7 {
		t.Fatalf("want b(9) then d(7), got %+v", got)
	}
	if n := len(topReasons(map[string]int{}, 10)); n != 0 {
		t.Fatalf("empty input → empty output, got %d", n)
	}
}

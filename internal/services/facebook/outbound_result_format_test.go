package facebook

import (
	"strings"
	"testing"
)

// Characterization tests for the FB outbound-result copy formatters, moved with
// the helpers out of cmd/scraper (PR29F). They pin CURRENT behavior so the move
// is provably behavior-preserving — what the code does today, not what it "should".

// FormatOutboundNotification pins the operator-facing copy: the DEFAULT (non-auto)
// mode says "drafts waiting for approval" (approval-required is the visible
// default), names the channel by msgType, and surfaces org/account/queued/skipped.
func TestFormatOutboundNotification(t *testing.T) {
	const orgID, accountID = int64(7), int64(42)

	draft := FormatOutboundNotification(orgID, accountID, "comment", 3, 1, "draft")
	for _, want := range []string{
		"Facebook comments", "queued: 3", "drafts waiting for approval",
		"Org #7", "account #42", "skipped 1",
	} {
		if !strings.Contains(draft, want) {
			t.Errorf("draft notification missing %q, got %q", want, draft)
		}
	}

	// approved_auto mode flips ONLY the state clause to the execution-ready copy.
	approved := FormatOutboundNotification(orgID, accountID, "comment", 3, 0, "approved_auto")
	if !strings.Contains(approved, "approved for Chrome Extension execution") {
		t.Errorf("approved_auto must say execution-ready, got %q", approved)
	}
	if strings.Contains(approved, "drafts waiting for approval") {
		t.Errorf("approved_auto must NOT say drafts-waiting, got %q", approved)
	}

	// Channel label varies by msgType; everything else is shared copy.
	labels := map[string]string{
		"comment":      "Facebook comments",
		"inbox":        "Facebook inbox",
		"group_post":   "Facebook posting",
		"profile_post": "Facebook profile posting",
		"other":        "outbound", // unknown msgType falls back to the neutral label
	}
	for msgType, want := range labels {
		got := FormatOutboundNotification(orgID, accountID, msgType, 1, 0, "draft")
		if !strings.Contains(got, want) {
			t.Errorf("msgType %q label: want %q in %q", msgType, want, got)
		}
	}
}

// FriendlySkipReasons pins the forensics copy contract: each reason renders as
// "<friendly> [<raw_code>] ×<n>" so the exact gate is unambiguous, an unknown code
// degrades to "cần kiểm tra" (never crashes), and an empty map degrades honestly.
func TestFriendlySkipReasons(t *testing.T) {
	if got := FriendlySkipReasons(map[string]int{}); got != "không đủ điều kiện" {
		t.Errorf("empty reasons = %q, want %q", got, "không đủ điều kiện")
	}

	// Known reason: friendly label + bracketed raw code + count.
	known := FriendlySkipReasons(map[string]int{"missing_post_permalink": 2})
	if !strings.Contains(known, "[missing_post_permalink]") || !strings.Contains(known, "×2") {
		t.Errorf("known reason must keep the raw code + count, got %q", known)
	}

	// Unknown reason degrades to the generic label but still keeps the raw code.
	unknown := FriendlySkipReasons(map[string]int{"brand_new_gate": 1})
	if !strings.Contains(unknown, "cần kiểm tra") || !strings.Contains(unknown, "[brand_new_gate]") {
		t.Errorf("unknown reason must degrade gracefully + keep raw code, got %q", unknown)
	}
}

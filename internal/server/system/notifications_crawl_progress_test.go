package system

import (
	"strings"
	"testing"
)

// PR-C2 added two safe exit reasons (scroll_not_moving, duplicate_heavy). They
// must render as operator-friendly Vietnamese, existing labels must be unchanged,
// and an unknown code must still fall back to the raw string (no leak/crash).
func TestCrawlExitReasonLabel(t *testing.T) {
	cases := map[string]string{
		"scroll_not_moving":         "không cuộn thêm được bài mới",
		"duplicate_heavy":           "gặp nhiều bài trùng, không thấy bài mới",
		"no_new_items_after_scroll": "đã cuộn tiếp nhưng không thấy bài mới", // unchanged
		"no_progress":               "Facebook không tải thêm bài sau nhiều lần cuộn", // unchanged
		"MaxItems":                  "đã đạt số bài yêu cầu",                          // exercises case-insensitive matching
		"something_unknown":         "something_unknown",                               // default → raw code
	}
	for reason, want := range cases {
		if got := crawlExitReasonLabel(reason); got != want {
			t.Errorf("crawlExitReasonLabel(%q) = %q, want %q", reason, got, want)
		}
	}
}

// PR-C1B: the compact diagnostic suffix must (1) stay empty for a pre-C1B
// payload so old extensions read exactly as before, (2) render a human-safe
// pause message for risk codes WITHOUT leaking raw page text, and (3) show the
// compact phase/no-progress/duplicate line otherwise.
func TestCrawlProgressDiagVN(t *testing.T) {
	// Backward compatibility: no phase, no reason → no suffix at all.
	if got := crawlProgressDiagVN(CrawlProgressNotice{}); got != "" {
		t.Fatalf("pre-C1B payload must yield empty diag suffix, got %q", got)
	}

	// Risk codes → the fixed safety message; never continues/bypasses.
	for _, code := range []string{"login_required", "checkpoint_suspected", "risk_blocked"} {
		got := crawlProgressDiagVN(CrawlProgressNotice{SafeReasonCode: code, Phase: "blocked"})
		if !strings.Contains(got, "tạm dừng") || !strings.Contains(got, "checkpoint") {
			t.Errorf("risk code %q must render the pause message, got %q", code, got)
		}
		if !strings.Contains(got, "Không tự xử lý") {
			t.Errorf("risk code %q must state no auto-resolution, got %q", code, got)
		}
	}

	// Normal progress diagnostics → Vietnamese phase + only the non-zero
	// counters. "Vòng không tăng tiến độ" (scroll rounds that moved nothing)
	// prints because it is 6 here; the phase label is Vietnamese, never the raw
	// machine word.
	got := crawlProgressDiagVN(CrawlProgressNotice{Phase: "stalled", NoProgressRounds: 6, DuplicateCount: 12, SafeReasonCode: "no_new_posts"})
	want := " Pha: không tăng tiến độ. Vòng không tăng tiến độ: 6. Bài trùng: 12."
	if got != want {
		t.Errorf("diag suffix = %q, want %q", got, want)
	}

	// duplicate_heavy: Vietnamese phase, NO contradictory zero counter, and the
	// explicit signal sentence so the operator reads "likely out of new posts".
	got = crawlProgressDiagVN(CrawlProgressNotice{Phase: "stalled", NoProgressRounds: 0, DuplicateCount: 78, SafeReasonCode: "duplicate_heavy"})
	want = " Pha: nhiều bài trùng. Bài trùng: 78. Tín hiệu: nhiều bài trùng, có thể đã hết bài mới."
	if got != want {
		t.Errorf("duplicate_heavy diag suffix = %q, want %q", got, want)
	}
	if strings.Contains(got, "stalled") {
		t.Errorf("Vietnamese message must not leak the raw machine phase, got %q", got)
	}
	if strings.Contains(got, ": 0") {
		t.Errorf("zero counters must be omitted, got %q", got)
	}

	// A healthy scrolling heartbeat keeps a compact Vietnamese phase and prints
	// no zero counters at all.
	got = crawlProgressDiagVN(CrawlProgressNotice{Phase: "scrolling", SafeReasonCode: "scrolling"})
	if want = " Pha: đang quét."; got != want {
		t.Errorf("scrolling diag suffix = %q, want %q", got, want)
	}
}

// A risk-code suffix must not embed any raw page/DOM text — only the fixed,
// translated safety sentence. Guards the "no raw checkpoint text" invariant.
func TestCrawlProgressDiagVNNoRawText(t *testing.T) {
	got := crawlProgressDiagVN(CrawlProgressNotice{SafeReasonCode: "checkpoint_suspected"})
	// The message is a fixed constant; assert it equals the whole suffix so a
	// future change that interpolates page text into it fails here.
	want := " Đã tạm dừng: cần kiểm tra đăng nhập/checkpoint. Không tự xử lý checkpoint."
	if got != want {
		t.Errorf("checkpoint suffix must be the fixed safe sentence, got %q", got)
	}
}

package system

import (
	"strings"
	"testing"
)

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

	// Normal progress diagnostics → compact phase/no-progress/duplicate suffix.
	got := crawlProgressDiagVN(CrawlProgressNotice{Phase: "stalled", NoProgressRounds: 6, DuplicateCount: 12, SafeReasonCode: "no_new_posts"})
	for _, want := range []string{"Pha: stalled", "6 vòng", "Trùng: 12"} {
		if !strings.Contains(got, want) {
			t.Errorf("diag suffix missing %q, got %q", want, got)
		}
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

package copilot

import (
	"strings"
	"testing"
)

// promptIsSelfSufficient is the gate that decides whether the orchestrator
// can bypass the brain planner. A regression here is the over-defensive-
// gating bug returning: it would cause "configure your business profile"
// ask-backs for prompts that already carry full intent.
//
// Two truth invariants:
//   - YES when URL + crawl verb + (count OR inferred signals).
//   - NO when ambiguous (no URL, or no crawl verb, or neither count nor signals).
func TestPromptIsSelfSufficient(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{
			name:   "the user's reported case — URL + count + buyer signals",
			prompt: "Cào cho tôi 50 bài liên quan đến tệp seller có nhu cầu fulfill POD,dropship trong https://www.facebook.com/groups/12345",
			want:   true,
		},
		{
			name:   "URL + crawl verb + explicit count (no signals)",
			prompt: "crawl 50 posts in https://facebook.com/groups/12345",
			want:   true,
		},
		{
			name:   "URL + crawl verb + buyer signals (no count)",
			prompt: "crawl https://facebook.com/groups/12345 to find sellers needing fulfillment dropship",
			want:   true,
		},
		{
			name:   "ambiguous: no URL — should fall to brain",
			prompt: "find me 50 customers looking for POD fulfillment",
			want:   false,
		},
		{
			name:   "ambiguous: URL but no crawl verb — research question",
			prompt: "what is https://facebook.com/groups/12345 about?",
			want:   false,
		},
		{
			name:   "ambiguous: URL + crawl verb but no count + no inferred signals",
			prompt: "crawl https://facebook.com/groups/12345",
			want:   false,
		},
		{
			name:   "Vietnamese crawl verb with diacritics",
			prompt: "Quét 30 bài trong https://facebook.com/groups/12345 tìm shop cần fulfillment",
			want:   true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := promptIsSelfSufficient(c.prompt); got != c.want {
				t.Errorf("promptIsSelfSufficient(%q) = %v; want %v", c.prompt, got, c.want)
			}
		})
	}
}

// inferredTargetingSummary is the trust-building UX response line. The
// shape MUST stay stable: operators rely on "Đối tượng nhận diện:" to
// confirm the system understood without re-reading the whole prompt.
// Empty string is the explicit "nothing inferred" signal.
func TestInferredTargetingSummary(t *testing.T) {
	t.Parallel()

	// Buyer-intent prompt — should include role label + matched signals.
	summary := inferredTargetingSummary("Cào 50 bài tìm seller cần fulfill POD dropship")
	if !strings.Contains(summary, "Đối tượng nhận diện:") {
		t.Errorf("missing audience label in summary: %q", summary)
	}
	if !strings.Contains(summary, "buyer-intent") {
		t.Errorf("buyer-intent prompt should produce buyer label; got %q", summary)
	}
	if !strings.Contains(summary, "Tín hiệu khớp:") {
		t.Errorf("missing matched-signals line; got %q", summary)
	}
	// Provider/spam filters surface as the "Lọc bỏ:" line so the operator
	// sees what we're EXCLUDING, not just including.
	if !strings.Contains(summary, "Lọc bỏ:") {
		t.Errorf("missing negative-signals line; got %q", summary)
	}

	// Candidate / hiring prompt — role label flips.
	hr := inferredTargetingSummary("crawl 30 bài tuyển dụng senior backend developer")
	if !strings.Contains(hr, "ứng viên") {
		t.Errorf("hiring prompt should produce candidate label; got %q", hr)
	}

	// Empty prompt → empty summary (no spurious "Đối tượng nhận diện: ").
	if got := inferredTargetingSummary(""); got != "" {
		t.Errorf("empty prompt should produce empty summary; got %q", got)
	}
}

// promptIsLeadActionSelfSufficient is the second self-sufficiency gate:
// it covers outbound actions on already-stored leads (comment / inbox /
// DM "all leads"). The regression here is the same family of bug as
// promptIsSelfSufficient — brain.py was bouncing these prompts into an
// ask-back for business positioning even when the workspace already had
// qualified leads ready to act on.
//
// Invariants:
//   - YES when outbound-action verb + "all leads" scope phrase + no URL.
//   - NO when a URL is present (that is the crawl path, not the leads
//     pool path) or when either side of the verb+scope conjunction is
//     missing.
func TestPromptIsLeadActionSelfSufficient(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{
			name:   "the user's reported case — Vietnamese comment-on-all-leads",
			prompt: "Comments lên tất cả các leads cho tôi",
			want:   true,
		},
		{
			name:   "English inbox-all-leads",
			prompt: "Inbox all leads",
			want:   true,
		},
		{
			name:   "Vietnamese inbox phrasing",
			prompt: "Nhắn tin tất cả khách hàng đủ điều kiện",
			want:   true,
		},
		{
			name:   "DM with scope phrase",
			prompt: "DM tất cả leads ngay",
			want:   true,
		},
		{
			name:   "outbound verb but no scope phrase — single-target",
			prompt: "Comment cho bài này thôi",
			want:   false,
		},
		{
			name:   "scope phrase but no outbound verb — could be a report request",
			prompt: "Xem tất cả các leads",
			want:   false,
		},
		{
			name:   "URL present — this is scrape_comments / crawl path, not leads-pool",
			prompt: "Comment giúp tôi trong https://facebook.com/groups/12345",
			want:   false,
		},
		{
			name:   "crawl prompt without URL — must NOT trip lead-action gate",
			prompt: "Cào 50 bài tìm tệp khách POD",
			want:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := promptIsLeadActionSelfSufficient(c.prompt); got != c.want {
				t.Errorf("promptIsLeadActionSelfSufficient(%q) = %v; want %v", c.prompt, got, c.want)
			}
		})
	}
}

// Regression guard: the wiring at agent_responses.go calls
// inferredTargetingSummary(prompt) — make sure the helper doesn't
// silently start panicking on prompts with no inferrable signals.
func TestInferredTargetingSummary_NoSignalSafe(t *testing.T) {
	t.Parallel()
	// Bare prompt with no targeting hints — current behaviour: the role
	// default is "customers" which still produces a summary. That's
	// acceptable; the summary just says "khách hàng tiềm năng" with no
	// matched signals. If a future change wants to suppress this, update
	// inferCrawlTargetingFromPrompt to not default the role.
	got := inferredTargetingSummary("crawl https://facebook.com/groups/12345")
	if got == "" {
		// Acceptable: if the helper learns to suppress role-only summaries.
		return
	}
	if !strings.Contains(got, "Đối tượng nhận diện:") {
		t.Errorf("non-empty summary must carry the audience label; got %q", got)
	}
}

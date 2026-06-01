package runtime

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// ClassifyExtensionReport is the wire between Chrome-extension outcome
// reports and the unified ExecutionOutcome taxonomy. Test the full
// matrix because each branch produces a different downstream signal
// (badge, ledger, risk_score). Misclassification = hallucinated state.

func TestClassifyExtensionReport_StrongCommentProof_DOMVerified(t *testing.T) {
	out, _ := ClassifyExtensionReport(ExtensionExecutionReport{
		Success:          true,
		CommentPermalink: "https://facebook.com/x?comment_id=999",
		NodeMatched:      true,
		CountIncreased:   true,
	})
	if out != models.ExecutionDOMVerified {
		t.Errorf("strong comment proof → %q; want dom_verified", out)
	}
}

func TestClassifyExtensionReport_StrongInboxProof_DOMVerified(t *testing.T) {
	out, _ := ClassifyExtensionReport(ExtensionExecutionReport{
		Success:         true,
		MessageBubbleID: "row_42",
		BubbleFresh:     true,
	})
	if out != models.ExecutionDOMVerified {
		t.Errorf("strong inbox proof → %q; want dom_verified", out)
	}
}

// Success=true with no DOM proof is the legacy /sent contract. We must
// NOT promote it to dom_verified; only optimistic_success. The IsSuccessOutcome
// guard still treats it as success for the badge (backward-compatible UI),
// but it does NOT lower risk_score — the orchestrator must be told the
// proof was missing.
func TestClassifyExtensionReport_LegacyNoBodyAssertion_Optimistic(t *testing.T) {
	out, proof := ClassifyExtensionReport(ExtensionExecutionReport{Success: true})
	if out != models.ExecutionOptimisticSuccess {
		t.Errorf("no proof → %q; want optimistic_success", out)
	}
	if !strings.Contains(proof.Notes, "without strong DOM proof") {
		t.Errorf("optimistic outcome should record a 'no strong proof' note; got %q", proof.Notes)
	}
}

// Duplicate-detected wins over weak-proof success — the executor saw the
// action already there pre-submit, so we record idempotently as duplicate.
func TestClassifyExtensionReport_Duplicate_Wins(t *testing.T) {
	out, _ := ClassifyExtensionReport(ExtensionExecutionReport{
		Success:   true,
		Duplicate: true,
	})
	if out != models.ExecutionDuplicateBlocked {
		t.Errorf("duplicate → %q; want duplicate_blocked", out)
	}
}

// failure_reason mapping covers the named failure modes. Unknown reasons
// fall back to shadow_rejected — the safe default that flags the row
// rather than letting it silently claim success.
func TestClassifyExtensionReport_FailureReasonMapping(t *testing.T) {
	cases := []struct {
		reason string
		want   models.ExecutionOutcome
	}{
		{"captcha", models.ExecutionCaptcha},
		{"rate_limited", models.ExecutionRateLimited},
		{"rate_limit", models.ExecutionRateLimited},
		{"blocked", models.ExecutionBlocked},
		{"post_deleted", models.ExecutionBlocked},
		{"muted", models.ExecutionBlocked},
		{"redirect", models.ExecutionRedirectedFeed},
		{"redirected_feed", models.ExecutionRedirectedFeed},
		{"feed_escape", models.ExecutionRedirectedFeed},
		{"composer_failed", models.ExecutionComposerFailed},
		{"composer", models.ExecutionComposerFailed},
		{"soft_fail", models.ExecutionSoftFail},
		{"transient", models.ExecutionSoftFail},
		{"network", models.ExecutionSoftFail},
		{"context_drift", models.ExecutionContextDrift},
		// Inbox message-request folder: sender-side bubble rendered but
		// recipient is non-connected. Must NOT be a success-class outcome
		// (would falsely promote the lead to protected); maps to the safe
		// shadow_rejected class.
		{"message_request_folder", models.ExecutionShadowRejected},
		// Unknown — never silently default to success-class.
		{"who_knows", models.ExecutionShadowRejected},
		{"", models.ExecutionShadowRejected},
	}
	for _, c := range cases {
		out, _ := ClassifyExtensionReport(ExtensionExecutionReport{
			Success:       false,
			FailureReason: c.reason,
		})
		if out != c.want {
			t.Errorf("FailureReason=%q → %q; want %q", c.reason, out, c.want)
		}
	}
}

// B1: a message-request-folder inbox delivery must NOT be treated as a
// verified touch (else the lead is wrongly marked protected/followup), and
// the granular diagnostic note must survive into proof.Notes so it reaches
// the operator's chat (A1 end-to-end contract).
func TestClassifyExtensionReport_MessageRequestFolder_NotSuccess(t *testing.T) {
	out, proof := ClassifyExtensionReport(ExtensionExecutionReport{
		Success:       false,
		FailureReason: "message_request_folder",
		Notes:         "inbox.message_request_folder: bubble rendered but recipient appears non-connected (matched: Tin nhắn chờ)",
	})
	if out != models.ExecutionShadowRejected {
		t.Errorf("message_request_folder → %q; want shadow_rejected", out)
	}
	if models.IsSuccessOutcome(out) {
		t.Errorf("message_request_folder must not be a success outcome (would falsely mark the lead as a verified touch)")
	}
	if proof.Notes == "" || !strings.Contains(proof.Notes, "message_request_folder") {
		t.Errorf("granular note dropped; got proof.Notes=%q", proof.Notes)
	}
}

// Proof field truncation guards against an over-chatty executor flooding
// the evidence_json column. 2KB cap is enforced by the classifier so the
// store layer doesn't need its own bound.
func TestClassifyExtensionReport_DOMSnippetTruncated(t *testing.T) {
	bigSnippet := strings.Repeat("x", 5000)
	_, proof := ClassifyExtensionReport(ExtensionExecutionReport{
		Success:          true,
		CommentPermalink: "https://fb.com/x?comment_id=1",
		NodeMatched:      true,
		DOMSnippet:       bigSnippet,
	})
	if len(proof.DOMSnippet) > 2200 { // 2048 + truncate marker
		t.Errorf("DOMSnippet not bounded; len=%d", len(proof.DOMSnippet))
	}
	if !strings.HasSuffix(proof.DOMSnippet, "[truncated]") {
		t.Errorf("truncation marker missing")
	}
}

package finalize

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store"
)

// TestProofNotesReachesOperatorMessage proves the COMPLETE A1 data path end
// to end — the exact chain the founder asked to confirm before running a
// production test:
//
//	extension report.Notes
//	  → runtime.ClassifyExtensionReport   (copies report.Notes verbatim)
//	  → runtime.EnforceTargetIdentity     (only APPENDS, never overwrites)
//	  → agent.notificationDetail          (prefers proof.Notes over coarse fallbacks)
//	  → system.NotifyOutboundStatusDetail (renders the operator message)
//	  → captured notifier                 (the chat/Telegram sink)
//
// The granular "path2.article_not_found_in_feed" prefix must arrive intact in
// the final string handed to the operator. If ANY hop overwrote it with
// report.FailureReason or string(outcome), this test fails — which is the
// founder's definition of "A1 not done".
func TestProofNotesReachesOperatorMessage(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "notif.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer db.Close()

	const orgID = int64(5)
	const targetURL = "https://www.facebook.com/groups/1312868109620530/posts/2031780854395915/"
	id, err := db.Outbound().Insert(&models.OutboundMessage{
		OrgID:      orgID,
		Type:       "comment",
		Platform:   "facebook",
		AccountID:  49,
		TargetURL:  targetURL,
		TargetName: "Yến Nhi · Follow",
		Content:    "xin chào",
	})
	if err != nil {
		t.Fatalf("InsertOutboundMessage: %v", err)
	}

	// The exact proof shape executeCommentInFeed emits for a Path-2
	// article-not-found failure: failure_reason=context_drift, and notes
	// carrying the granular path prefix (buildCommentProof seeds
	// "context_drift", then commentResult appends the path2 note → the
	// substring is present even though it is not at index 0).
	report := runtime.ExtensionExecutionReport{
		Success:       false,
		FailureReason: "context_drift",
		Notes:         "context_drift · path2.article_not_found_in_feed: target id=2031780854395915 scrolls=8 articles_in_dom=12 landed_at=https://www.facebook.com/groups/1312868109620530/",
		PageURLAfter:  "https://www.facebook.com/groups/1312868109620530/",
		ExecutionID:   "exec-test",
	}

	// Run the REAL backend pipeline — identical calls to finalizeOutbound.
	outcome, proof := runtime.ClassifyExtensionReport(report)
	outcome, proof = runtime.EnforceTargetIdentity(outcome, proof, targetURL, "comment")
	detail := notificationDetail(proof, report, outcome)

	// The granular path note is still preserved on the detail (→ evidence + server
	// log). Execution Visibility (#8) moved the RAW code OUT of the operator chat,
	// but the detail path itself must never be dropped upstream.
	if !strings.Contains(detail, "path2.article_not_found_in_feed") {
		t.Fatalf("notificationDetail dropped the path note; got %q", detail)
	}

	terminalState, terminalOutcome := models.TerminalFromOutcome(outcome)
	if models.IsVerifiedSuccess(terminalState, terminalOutcome) {
		t.Fatalf("article-not-found classified as verified success (state=%q outcome=%q)", terminalState, terminalOutcome)
	}

	var captured string
	notifier := func(msg string) { captured = msg }
	system.NotifyOutboundStatusDetail(db, notifier, orgID, id, terminalState, terminalOutcome, detail)

	if captured == "" {
		t.Fatal("notifier never fired — the operator would see nothing in chat")
	}
	// Execution Visibility (#4/#8): the operator chat shows a BUSINESS-FRIENDLY
	// reason, never the raw code. context_drift → "Facebook chuyển trang…", and the
	// raw "path2…" / "context_drift" token must NOT leak into the chat string.
	if !strings.Contains(captured, "Facebook chuyển trang trước khi gửi comment") {
		t.Fatalf("operator message missing the friendly reason.\n got: %q", captured)
	}
	if strings.Contains(captured, "path2") || strings.Contains(captured, "context_drift") {
		t.Fatalf("operator message leaked the raw reason code.\n got: %q", captured)
	}
	if !strings.Contains(captured, "thất bại") || !strings.Contains(captured, "Yến Nhi") {
		t.Fatalf("operator message missing failure framing or lead name.\n got: %q", captured)
	}
	t.Logf("operator chat message: %s", captured)
}

// TestNotificationDetail_FallbackOrder pins the selection precedence so a
// future refactor cannot silently regress A1 back to surfacing the coarse
// outcome when a granular note exists.
func TestNotificationDetail_FallbackOrder(t *testing.T) {
	cases := []struct {
		name    string
		proof   runtime.VerifierProof
		report  runtime.ExtensionExecutionReport
		outcome models.ExecutionOutcome
		want    string
	}{
		{
			name:    "prefers proof.Notes",
			proof:   runtime.VerifierProof{Notes: "path2.group_home_nav_failed: target_group=…"},
			report:  runtime.ExtensionExecutionReport{FailureReason: "redirected_feed"},
			outcome: models.ExecutionRedirectedFeed,
			want:    "path2.group_home_nav_failed: target_group=…",
		},
		{
			name:    "falls back to failure_reason when no note",
			proof:   runtime.VerifierProof{Notes: ""},
			report:  runtime.ExtensionExecutionReport{FailureReason: "rate_limited"},
			outcome: models.ExecutionRateLimited,
			want:    "rate_limited",
		},
		{
			name:    "falls back to outcome when neither present",
			proof:   runtime.VerifierProof{Notes: "  "},
			report:  runtime.ExtensionExecutionReport{FailureReason: ""},
			outcome: models.ExecutionShadowRejected,
			want:    "shadow_rejected",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := notificationDetail(c.proof, c.report, c.outcome); got != c.want {
				t.Errorf("notificationDetail = %q; want %q", got, c.want)
			}
		})
	}
}

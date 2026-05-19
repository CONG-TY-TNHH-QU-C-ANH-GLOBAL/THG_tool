package runtime

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// TestEnforceTargetIdentity_SamePost_Pass covers every URL shape pair
// the production verifier will see. Each pair MUST normalise to the
// same canonical entity id so the executor's success-class outcome
// flows through unchanged.
func TestEnforceTargetIdentity_SamePost_Pass(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		target, actual string
	}{
		{
			name:   "numeric group post — actual carries comment_id anchor",
			target: "https://www.facebook.com/groups/1312868109620530/posts/2019673682273299/",
			actual: "https://www.facebook.com/groups/1312868109620530/posts/2019673682273299/?comment_id=999",
		},
		{
			name:   "pfbid profile post — actual carries comment_id anchor",
			target: "https://www.facebook.com/luong.the.hung.800599/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl",
			actual: "https://www.facebook.com/luong.the.hung.800599/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl?comment_id=1293405342441584",
		},
		{
			name:   "story_fbid query == /posts/ numeric",
			target: "https://www.facebook.com/groups/X/posts/2019673682273299/",
			actual: "https://www.facebook.com/story.php?story_fbid=2019673682273299&id=42",
		},
		{
			name:   "/permalink/ form == /posts/ numeric",
			target: "https://www.facebook.com/groups/X/permalink/2019673682273299/",
			actual: "https://www.facebook.com/groups/X/posts/2019673682273299/",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proof := VerifierProof{PageURLAfter: c.actual}
			outcome, _ := EnforceTargetIdentity(models.ExecutionDOMVerified, proof, c.target, "comment")
			if outcome != models.ExecutionDOMVerified {
				t.Errorf("same entity must preserve outcome; got %q want %q",
					outcome, models.ExecutionDOMVerified)
			}
		})
	}
}

// TestEnforceTargetIdentity_DifferentPost_Fail is the EXACT shape of the
// May-2026 incident. Target was a group post permalink; the extension
// reported success but page_url_after pointed at a different profile
// post with a comment_id anchor. The guard must downgrade to
// ContextDrift and annotate the proof so operator replay can see why.
func TestEnforceTargetIdentity_DifferentPost_Fail(t *testing.T) {
	t.Parallel()
	target := "https://www.facebook.com/groups/1312868109620530/posts/2019673682273299/"
	actual := "https://www.facebook.com/luong.the.hung.800599/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl?comment_id=1293405342441584"

	proof := VerifierProof{PageURLAfter: actual}
	outcome, gotProof := EnforceTargetIdentity(models.ExecutionDOMVerified, proof, target, "comment")
	if outcome != models.ExecutionContextDrift {
		t.Fatalf("mismatched entity must downgrade; got %q want %q",
			outcome, models.ExecutionContextDrift)
	}
	if !strings.Contains(gotProof.Notes, "entity-drift") {
		t.Errorf("proof.Notes must annotate drift; got %q", gotProof.Notes)
	}
	if !strings.Contains(gotProof.Notes, "target=") || !strings.Contains(gotProof.Notes, "actual=") {
		t.Errorf("annotation must include both target= and actual= for replay; got %q", gotProof.Notes)
	}
}

// TestEnforceTargetIdentity_MalformedTarget_FailClosed exercises the
// fail-closed rule for the target side. If the queue produced a target
// without a parseable entity id, we cannot independently verify the
// destination — success-class outcomes must downgrade.
func TestEnforceTargetIdentity_MalformedTarget_FailClosed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, target string
	}{
		{"empty target", ""},
		{"target is a profile shell, no post id", "https://www.facebook.com/some.user"},
		{"target is garbage", "this is not a url"},
		{"target is a group root with no /posts/", "https://www.facebook.com/groups/1312868109620530/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proof := VerifierProof{
				PageURLAfter: "https://www.facebook.com/groups/X/posts/2019673682273299/",
			}
			outcome, gotProof := EnforceTargetIdentity(models.ExecutionDOMVerified, proof, c.target, "comment")
			if outcome != models.ExecutionContextDrift {
				t.Fatalf("malformed target must fail closed; got %q want %q",
					outcome, models.ExecutionContextDrift)
			}
			if !strings.Contains(gotProof.Notes, "entity-drift") {
				t.Errorf("proof.Notes must annotate drift; got %q", gotProof.Notes)
			}
		})
	}
}

// TestEnforceTargetIdentity_MalformedActual_FailClosed mirrors the
// malformed-target test for the page_url_after side. An extension that
// omits or garbles page_url_after has not given us enough evidence to
// corroborate a "sent" claim. Defense-in-depth = refuse to verify.
func TestEnforceTargetIdentity_MalformedActual_FailClosed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, actual string
	}{
		{"empty page_url_after", ""},
		{"page_url_after is feed home", "https://www.facebook.com/"},
		{"page_url_after is garbage", "javascript:void(0)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proof := VerifierProof{PageURLAfter: c.actual}
			outcome, _ := EnforceTargetIdentity(
				models.ExecutionDOMVerified,
				proof,
				"https://www.facebook.com/groups/X/posts/2019673682273299/",
				"comment",
			)
			if outcome != models.ExecutionContextDrift {
				t.Fatalf("malformed actual must fail closed; got %q want %q",
					outcome, models.ExecutionContextDrift)
			}
		})
	}
}

// TestEnforceTargetIdentity_FailureOutcome_Preserved guarantees the
// guard NEVER promotes an outcome and NEVER reclassifies failure
// outcomes. Re-labelling a rate_limited action as context_drift would
// destroy the risk-signal pipeline's specific failure information.
func TestEnforceTargetIdentity_FailureOutcome_Preserved(t *testing.T) {
	t.Parallel()
	failures := []models.ExecutionOutcome{
		models.ExecutionRateLimited,
		models.ExecutionBlocked,
		models.ExecutionRedirectedFeed,
		models.ExecutionShadowRejected,
		models.ExecutionCaptcha,
		models.ExecutionContextDrift, // already drift — must not double-annotate
	}
	for _, in := range failures {
		t.Run(string(in), func(t *testing.T) {
			proof := VerifierProof{PageURLAfter: ""}
			outcome, _ := EnforceTargetIdentity(in, proof, "", "comment")
			if outcome != in {
				t.Errorf("failure outcome must pass through unchanged; got %q want %q", outcome, in)
			}
		})
	}
}

// TestEnforceTargetIdentity_NonCommentActions_Skipped — inbox and
// post types have different identity recipes and live in their own
// guards (future PRs). The comment-only guard MUST be a no-op for
// other types so it doesn't silently expand its responsibility surface.
func TestEnforceTargetIdentity_NonCommentActions_Skipped(t *testing.T) {
	t.Parallel()
	cases := []string{"inbox", "group_post", "profile_post", "", "unknown_type"}
	for _, action := range cases {
		t.Run(action, func(t *testing.T) {
			// Mismatched URLs that WOULD trigger drift for a comment.
			proof := VerifierProof{PageURLAfter: "https://www.facebook.com/groups/X/posts/111/"}
			outcome, _ := EnforceTargetIdentity(
				models.ExecutionDOMVerified,
				proof,
				"https://www.facebook.com/groups/X/posts/222/",
				action,
			)
			if outcome != models.ExecutionDOMVerified {
				t.Errorf("non-comment action %q must skip the guard; got %q", action, outcome)
			}
		})
	}
}

// TestEnforceTargetIdentity_PfbidNormalization spot-checks that the
// pfbid token survives query-string truncation. The incident URL had
// ?comment_id=… appended; the actual id must still extract.
func TestEnforceTargetIdentity_PfbidNormalization(t *testing.T) {
	t.Parallel()
	target := "https://www.facebook.com/u/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl"
	actual := target + "?comment_id=1293405342441584"

	proof := VerifierProof{PageURLAfter: actual}
	outcome, _ := EnforceTargetIdentity(models.ExecutionDOMVerified, proof, target, "comment")
	if outcome != models.ExecutionDOMVerified {
		t.Errorf("pfbid with comment_id anchor must match base pfbid; got %q", outcome)
	}
}

// TestEnforceTargetIdentity_OptimisticSuccess_AlsoEnforced verifies the
// guard runs on OptimisticSuccess too (it's a success class). An
// extension that reports optimistic_success WITHOUT a corroborating
// page_url_after match should still be downgraded — we don't get to
// hide behind "we were uncertain anyway".
func TestEnforceTargetIdentity_OptimisticSuccess_AlsoEnforced(t *testing.T) {
	t.Parallel()
	proof := VerifierProof{PageURLAfter: "https://www.facebook.com/groups/X/posts/111/"}
	outcome, _ := EnforceTargetIdentity(
		models.ExecutionOptimisticSuccess,
		proof,
		"https://www.facebook.com/groups/X/posts/222/",
		"comment",
	)
	if outcome != models.ExecutionContextDrift {
		t.Fatalf("optimistic_success on wrong post must downgrade; got %q", outcome)
	}
}

// TestEnforceTargetIdentity_NotesAnnotationFormat pins the annotation
// shape so operator-replay parsers / dashboards can rely on it. The
// "entity-drift target=… actual=…" format is what the dashboard greps.
func TestEnforceTargetIdentity_NotesAnnotationFormat(t *testing.T) {
	t.Parallel()
	proof := VerifierProof{
		PageURLAfter: "https://www.facebook.com/u/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl",
		Notes:        "pre-existing note",
	}
	_, gotProof := EnforceTargetIdentity(
		models.ExecutionDOMVerified,
		proof,
		"https://www.facebook.com/groups/X/posts/2019673682273299/",
		"comment",
	)
	// Pre-existing note is preserved; annotation is appended with the
	// dot-separator the operator-replay UI uses for multi-fact notes.
	if !strings.HasPrefix(gotProof.Notes, "pre-existing note · entity-drift ") {
		t.Errorf("annotation should be appended with ' · ' separator; got %q", gotProof.Notes)
	}
	if !strings.Contains(gotProof.Notes, `target="2019673682273299"`) {
		t.Errorf("target id should appear quoted in annotation; got %q", gotProof.Notes)
	}
	// Long pfbid is abbreviated to first 16 chars + ellipsis to bound
	// the evidence_json column footprint.
	if !strings.Contains(gotProof.Notes, `actual="pfbid02R3qUXYGfC…"`) {
		t.Errorf("long actual id should be abbreviated; got %q", gotProof.Notes)
	}
}

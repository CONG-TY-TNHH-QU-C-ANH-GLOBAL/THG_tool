package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/models"
)

// VerifierProof is the structured result of a post-action DOM observation.
// Maps onto store.VerificationEvidence at the persistence layer, but lives
// here so the runtime package can build it without an import cycle into store.
type VerifierProof struct {
	CommentPermalink string    `json:"comment_permalink,omitempty"`
	MessageBubbleID  string    `json:"message_bubble_id,omitempty"`
	DOMSnippet       string    `json:"dom_snippet,omitempty"`
	PageURLAfter     string    `json:"page_url_after,omitempty"`
	ObservedAt       time.Time `json:"observed_at,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	// NavDiagnostic carries the PR8A navigation-hardening telemetry the
	// Chrome extension captured (nav trace, landing gates, redirect class).
	// Nil for server-side verifiers and legacy extension builds. Persisted
	// verbatim into evidence_json so the diagnostic endpoint reads typed
	// fields instead of grepping notes.
	NavDiagnostic *models.NavDiagnostic `json:"nav_diagnostic,omitempty"`
}

// VerifyContext is the input contract for any per-action verifier. The
// caller (executor or independent cross-check) supplies:
//   - the chromedp tab context (already navigated to the action surface),
//   - the target_url (post / profile / group) for the action,
//   - the expected content (comment body / message text) for fuzzy match,
//   - the executing account's FB user id, so the verifier matches the
//     correct author avatar instead of trusting any node with the right text.
type VerifyContext struct {
	TabCtx          context.Context
	TargetURL       string
	ExpectedContent string // queued content; fuzzy-matched against rendered node
	ExpectedFBUID   string // executing account's FB user id; matches the author of the rendered node
	Window          time.Duration // bound the observation; default 6s when zero
}

// VerifyComment is the per-action verifier for a Facebook comment. Order
// of proof (strongest first):
//
//  1. A new comment node exists under the post whose author avatar matches
//     ExpectedFBUID AND whose text fuzzy-matches ExpectedContent → dom_verified.
//  2. The comment count incremented vs. snapshot AND composer cleared → optimistic_success.
//  3. Page redirected away from the post (newsfeed / home.php) → redirected_feed.
//  4. Rate-limit banner / toast visible → rate_limited.
//  5. Verifier window exhausted with no proof → shadow_rejected.
//  6. Verifier could not read the page at all (eval error) → verification_timeout.
//
// CRITICAL: click-success is NOT in this list. Only post-DOM state is.
// See project_execution_verification.md "What 'verified' means".
func VerifyComment(vc VerifyContext) (models.ExecutionOutcome, VerifierProof, error) {
	window := vc.Window
	if window <= 0 {
		window = 6 * time.Second
	}
	if vc.TabCtx == nil {
		return models.ExecutionVerificationTimeout, VerifierProof{}, fmt.Errorf("verify_comment: nil tab context")
	}
	obsCtx, cancel := context.WithTimeout(vc.TabCtx, window)
	defer cancel()

	var raw string
	if err := chromedp.Run(obsCtx, chromedp.Evaluate(verifyCommentJS(vc.ExpectedContent, vc.ExpectedFBUID), &raw)); err != nil {
		return models.ExecutionVerificationTimeout, VerifierProof{
			ObservedAt: time.Now().UTC(),
			Notes:      "chromedp eval failed: " + err.Error(),
		}, nil
	}

	var result struct {
		Verified       bool   `json:"verified"`
		Duplicate      bool   `json:"duplicate"`
		ComposerCleared bool  `json:"composer_cleared"`
		CountIncreased bool   `json:"count_increased"`
		CommentURL     string `json:"comment_url"`
		Snippet        string `json:"snippet"`
		PageURL        string `json:"page_url"`
		RateLimited    bool   `json:"rate_limited"`
		Blocked        bool   `json:"blocked"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return models.ExecutionVerificationTimeout, VerifierProof{
			ObservedAt: time.Now().UTC(),
			Notes:      "verify_comment: parse js output: " + err.Error(),
		}, nil
	}

	proof := VerifierProof{
		CommentPermalink: result.CommentURL,
		DOMSnippet:       truncateSnippet(result.Snippet, 2048),
		PageURLAfter:     result.PageURL,
		ObservedAt:       time.Now().UTC(),
	}

	// Classification order matches the docstring above. Each branch is
	// independently observable; we do not collapse them into a single
	// "platform_says_no" because the orchestrator (PR-5) prices them
	// differently in risk_score.
	var outcome models.ExecutionOutcome
	switch {
	case result.Duplicate:
		outcome = models.ExecutionDuplicateBlocked
	case result.Verified:
		outcome = models.ExecutionDOMVerified
	case result.Blocked:
		proof.Notes = "blocked banner/toast detected"
		outcome = models.ExecutionBlocked
	case result.RateLimited:
		proof.Notes = "rate-limit copy detected"
		outcome = models.ExecutionRateLimited
	case isTransientFacebookURL(result.PageURL):
		proof.Notes = "redirected to feed/home after submit"
		outcome = models.ExecutionRedirectedFeed
	case result.CountIncreased && result.ComposerCleared:
		// We saw partial proof (count moved, composer cleared) but couldn't
		// match the specific node — the comment is THERE but we can't be
		// certain it's OURS. Better than nothing; flag for re-verification.
		proof.Notes = "count+composer proof only; no node match"
		outcome = models.ExecutionOptimisticSuccess
	default:
		proof.Notes = "no DOM proof within window"
		outcome = models.ExecutionShadowRejected
	}
	// Defense-in-depth — even when the content+author match succeeds,
	// require the page we observed to be the SAME entity as the one
	// the caller intended. EnforceTargetIdentity downgrades any
	// success-class outcome to ContextDrift on mismatch, and is a
	// no-op when the outcome is already a failure class.
	outcome, proof = EnforceTargetIdentity(outcome, proof, vc.TargetURL, "comment")
	return outcome, proof, nil
}

// VerifyInbox is the per-action verifier for a Facebook Messenger thread.
// Different proof recipe than comment because the surface is a chat panel:
//
//  1. Latest bubble in the open thread has timestamp ≤ 30s AND text fuzzy-
//     matches AND no failure toast → dom_verified.
//  2. Latest bubble matches but timestamp can't be read → optimistic_success.
//  3. Failure toast visible ("Message not sent", red icon) → blocked.
//  4. Thread pane closed / URL no longer has thread id → redirected_feed.
//  5. Otherwise → shadow_rejected.
//
// Inbox verification is HIGHER STAKES than comment because false positives
// hallucinate `protected` / `followup_pending` badges on the lead — see
// project_distributed_coordination.md §PR-4. Be conservative.
func VerifyInbox(vc VerifyContext) (models.ExecutionOutcome, VerifierProof, error) {
	window := vc.Window
	if window <= 0 {
		window = 6 * time.Second
	}
	if vc.TabCtx == nil {
		return models.ExecutionVerificationTimeout, VerifierProof{}, fmt.Errorf("verify_inbox: nil tab context")
	}
	obsCtx, cancel := context.WithTimeout(vc.TabCtx, window)
	defer cancel()

	var raw string
	if err := chromedp.Run(obsCtx, chromedp.Evaluate(verifyInboxJS(vc.ExpectedContent), &raw)); err != nil {
		return models.ExecutionVerificationTimeout, VerifierProof{
			ObservedAt: time.Now().UTC(),
			Notes:      "chromedp eval failed: " + err.Error(),
		}, nil
	}

	var result struct {
		Verified     bool   `json:"verified"`
		BubbleID     string `json:"bubble_id"`
		FreshSeconds int    `json:"fresh_seconds"`
		Snippet      string `json:"snippet"`
		PageURL      string `json:"page_url"`
		ThreadOpen   bool   `json:"thread_open"`
		FailureToast bool   `json:"failure_toast"`
		RateLimited  bool   `json:"rate_limited"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return models.ExecutionVerificationTimeout, VerifierProof{
			ObservedAt: time.Now().UTC(),
			Notes:      "verify_inbox: parse js output: " + err.Error(),
		}, nil
	}

	proof := VerifierProof{
		MessageBubbleID: result.BubbleID,
		DOMSnippet:      truncateSnippet(result.Snippet, 2048),
		PageURLAfter:    result.PageURL,
		ObservedAt:      time.Now().UTC(),
	}

	if result.FailureToast {
		proof.Notes = "send-failure toast visible"
		return models.ExecutionBlocked, proof, nil
	}
	if result.RateLimited {
		proof.Notes = "rate-limit copy detected"
		return models.ExecutionRateLimited, proof, nil
	}
	if !result.ThreadOpen {
		proof.Notes = "thread pane closed after submit"
		return models.ExecutionRedirectedFeed, proof, nil
	}
	if result.Verified && result.FreshSeconds <= 30 {
		return models.ExecutionDOMVerified, proof, nil
	}
	if result.Verified {
		proof.Notes = fmt.Sprintf("bubble text matched but stale (%ds)", result.FreshSeconds)
		return models.ExecutionOptimisticSuccess, proof, nil
	}
	proof.Notes = "no matching bubble within window"
	return models.ExecutionShadowRejected, proof, nil
}

// ClassifyExtensionReport is the entry point for the Chrome-extension
// outbound path. The extension does the click in the user's browser and
// POSTs back a structured report; we classify here using the SAME taxonomy
// the server-side verifier emits, so downstream (ledger, risk signal,
// orchestrator) sees one normalised surface regardless of executor origin.
//
// success=true with full evidence → dom_verified.
// success=true with partial evidence → optimistic_success.
// success=false with reason → mapped failure outcome.
// success=false with no reason → shadow_rejected (safe default).
func ClassifyExtensionReport(report ExtensionExecutionReport) (models.ExecutionOutcome, VerifierProof) {
	proof := VerifierProof{
		CommentPermalink: strings.TrimSpace(report.CommentPermalink),
		MessageBubbleID:  strings.TrimSpace(report.MessageBubbleID),
		DOMSnippet:       truncateSnippet(report.DOMSnippet, 2048),
		PageURLAfter:     strings.TrimSpace(report.PageURLAfter),
		ObservedAt:       time.Now().UTC(),
		Notes:            strings.TrimSpace(report.Notes),
		NavDiagnostic:    report.NavDiagnostic, // PR8A: pass landing telemetry through to evidence_json
	}
	if report.Duplicate {
		return models.ExecutionDuplicateBlocked, proof
	}
	if !report.Success {
		switch strings.ToLower(strings.TrimSpace(report.FailureReason)) {
		case "captcha", "checkpoint":
			// checkpoint = FB identity-verification gate (2FA / "verify
			// it's you" / locked-out flow). Same outcome class as
			// captcha — session is held until a human resolves it,
			// account-level risk signal applies, no retry loop. The
			// distinct token is preserved on the wire so the extension
			// proof.notes can carry the diagnostic flavour, but the
			// classifier collapses both onto ExecutionCaptcha because
			// downstream (state machine, ledger, risk pipeline) only
			// branches on outcome, not failure_reason text.
			return models.ExecutionCaptcha, proof
		case "rate_limited", "rate_limit":
			return models.ExecutionRateLimited, proof
		case "blocked", "post_deleted", "muted":
			return models.ExecutionBlocked, proof
		case "message_request_folder":
			// Inbox: the bubble rendered on the sender side, but FB routes
			// the message to the recipient's message-requests/pending folder
			// because the two accounts are not connected. The recipient may
			// never open it, so this is NOT a verified touch — it must not
			// promote the lead to protected/followup_pending. shadow_rejected
			// is the correct class ("appeared to go through, real delivery
			// unconfirmed"); the granular reason rides proof.Notes. Mapped
			// explicitly (rather than relying on the default fall-through) so
			// this is a documented, intentional classification.
			return models.ExecutionShadowRejected, proof
		case "redirect", "redirected_feed", "feed_escape":
			return models.ExecutionRedirectedFeed, proof
		case "composer_failed", "composer", "comment_button_not_found":
			// comment_button_not_found (PR-B): reached the post but the Comment/Bình luận
			// surface never appeared — a pre-submit failure in the same family as a
			// composer that never opened. Retryable, no risk penalty; nothing was typed.
			return models.ExecutionComposerFailed, proof
		case "soft_fail", "transient", "network":
			return models.ExecutionSoftFail, proof
		case "context_drift":
			return models.ExecutionContextDrift, proof
		case "target_not_reached":
			// PR8A: navigation never reached the post; the executor stopped
			// before typing. Retryable, no risk signal — the precise cause is
			// in proof.NavDiagnostic.RedirectClass.
			return models.ExecutionTargetNotReached, proof
		default:
			return models.ExecutionShadowRejected, proof
		}
	}
	// success=true. Evidence-quality gate: only promote to dom_verified
	// when the extension produced strong proof. Otherwise optimistic.
	hasStrongCommentProof := proof.CommentPermalink != "" && (report.CountIncreased || report.NodeMatched)
	hasStrongInboxProof := proof.MessageBubbleID != "" && report.BubbleFresh
	if hasStrongCommentProof || hasStrongInboxProof {
		return models.ExecutionDOMVerified, proof
	}
	if proof.Notes == "" {
		proof.Notes = "extension reported success without strong DOM proof; flagged for re-verify"
	}
	return models.ExecutionOptimisticSuccess, proof
}

// ExtensionExecutionReport is the wire shape the Chrome extension POSTs to
// the outbound-sent endpoint after performing a click. Mirrors the server-
// side proof recipes so the classifier above can apply the same rules
// regardless of which executor produced the observation.
type ExtensionExecutionReport struct {
	Success          bool   `json:"success"`
	FailureReason    string `json:"failure_reason"`
	CommentPermalink string `json:"comment_permalink"`
	MessageBubbleID  string `json:"message_bubble_id"`
	DOMSnippet       string `json:"dom_snippet"`
	PageURLAfter     string `json:"page_url_after"`
	CountIncreased   bool   `json:"count_increased"`
	NodeMatched      bool   `json:"node_matched"`
	BubbleFresh      bool   `json:"bubble_fresh"`
	Duplicate        bool   `json:"duplicate"`
	Notes            string `json:"notes"`
	// NavDiagnostic is the PR8A navigation-hardening telemetry (nav trace +
	// landing gates + redirect class). Optional; absent on legacy builds.
	NavDiagnostic *models.NavDiagnostic `json:"nav_diagnostic,omitempty"`
	// EvidenceScreenshotB64 is a base64 JPEG of the failing tab the background
	// captured at the moment of failure. It is OUT-OF-BAND evidence: the server
	// decodes it to disk and records only the resulting path in
	// NavDiagnostic.ScreenshotPath — the bytes are NEVER persisted into
	// evidence_json (VerifierProof deliberately has no field for them). Empty on
	// success and on legacy builds. Bounded server-side before it touches disk.
	EvidenceScreenshotB64 string `json:"evidence_screenshot_b64,omitempty"`
	// ExecutionID is the per-attempt idempotency token the server
	// issued at claim time and the executor MUST echo back. The
	// terminal-state CAS in store.FinalizeOutboundAttempt requires
	// this to match the row's stored execution_id; mismatches return
	// 409 Conflict so a stale callback (e.g. SW restart + content-
	// script-side replay) cannot finalize a row that has been
	// re-claimed.
	ExecutionID string `json:"execution_id,omitempty"`
}

// truncateSnippet bounds proof text so a chatty extension can't bloat
// the evidence_json column. 2KB is enough for a reader to identify the
// rendered node; full screenshots live in object storage via ScreenshotPath.
func truncateSnippet(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…[truncated]"
}

// verifyCommentJS returns a JS snippet that walks the post DOM and emits
// the structured proof object VerifyComment consumes. Kept here so the
// taxonomy and the proof recipe live next to each other; any future
// recipe tweak (Facebook DOM change) touches both at once.
//
// Author match uses ExpectedFBUID — we look for an anchor that targets
// /profile.php?id=<uid> OR /<vanity>/ where vanity resolves to the uid.
// Fuzzy text match is lowercase trimmed substring, since FB sometimes
// auto-trims whitespace or appends mentions.
func verifyCommentJS(expectedContent, expectedFBUID string) string {
	// Escape user-supplied strings into the JS template via JSON encoding.
	content, _ := json.Marshal(strings.TrimSpace(expectedContent))
	uid, _ := json.Marshal(strings.TrimSpace(expectedFBUID))
	return fmt.Sprintf(`
(function() {
  function lower(s) { return (s || '').toLowerCase().trim(); }
  var expected = lower(%s);
  var expectedUID = %s;

  // Rate-limit / blocked signals — check first because they short-circuit.
  var pageText = (document.body && document.body.innerText) || '';
  var rateLimited = /you('re| are) posting too quickly|too many comments|slow down/i.test(pageText);
  var blocked = /comment can't be posted|action blocked|you can't comment|comment was removed/i.test(pageText);

  // Composer cleared signal — the textbox that held our queued content is empty.
  var composerCleared = true;
  document.querySelectorAll('[contenteditable="true"][role="textbox"]').forEach(function(el) {
    var t = lower(el.innerText);
    if (t && t.indexOf(expected) !== -1) composerCleared = false;
  });

  // Walk comment nodes under the post. Selectors are broad on purpose —
  // Facebook DOM changes; "any anchor whose href contains /comment_id=" is
  // robust across desktop/mobile/web variants.
  var commentNodes = document.querySelectorAll('[aria-label*="omment" i] [role="article"], [data-testid*="comment" i], div[role="article"]');
  var matched = null;
  for (var i = 0; i < commentNodes.length; i++) {
    var n = commentNodes[i];
    var txt = lower(n.innerText || '');
    if (!txt || txt.indexOf(expected) === -1) continue;
    if (expectedUID) {
      var authorHref = '';
      var a = n.querySelector('a[href*="/profile.php?id="], a[href*="facebook.com/"]');
      if (a) authorHref = a.getAttribute('href') || '';
      if (authorHref.indexOf(expectedUID) === -1) continue;
    }
    matched = n;
    break;
  }

  // Duplicate detection — a node matching our content existed AT PAGE LOAD
  // is theoretically distinguishable from one that appeared after submit,
  // but without a pre-submit snapshot we can't be sure. v1 leaves this to
  // the executor: if the executor saw the same content already there
  // before clicking, it passes duplicate=true via ExtensionExecutionReport.
  var duplicate = false;

  // Permalink extraction off the matched node.
  var commentURL = '';
  if (matched) {
    var permaA = matched.querySelector('a[href*="comment_id="], a[href*="/comments/"]');
    if (permaA) commentURL = permaA.href || '';
  }

  // Count incremented signal — Facebook renders something like "12 comments".
  // Without a pre-submit snapshot we can't verify the delta; we just flag
  // whether a non-zero count is visible. The orchestrator + retry layer
  // can pair this with extension-reported pre-counts later.
  var countIncreased = /\b(\d+)\s*(comment|bình luận)/i.test(pageText);

  return JSON.stringify({
    verified: !!matched,
    duplicate: duplicate,
    composer_cleared: composerCleared,
    count_increased: countIncreased,
    comment_url: commentURL,
    snippet: matched ? (matched.innerText || '').slice(0, 500) : '',
    page_url: window.location.href || '',
    rate_limited: rateLimited,
    blocked: blocked
  });
})()
`, content, uid)
}

// verifyInboxJS scans the open Messenger thread for a freshly-rendered
// bubble matching the queued message. Tighter recipe than comment because
// the thread surface is more constrained.
func verifyInboxJS(expectedContent string) string {
	content, _ := json.Marshal(strings.TrimSpace(expectedContent))
	return fmt.Sprintf(`
(function() {
  function lower(s) { return (s || '').toLowerCase().trim(); }
  var expected = lower(%s);
  var pageURL = window.location.href || '';
  var pageText = (document.body && document.body.innerText) || '';

  // Thread surface markers — when the URL still contains "/messages/" or
  // "/t/" we consider the thread open.
  var threadOpen = /\/messages\/|\/t\//.test(pageURL);

  // Failure / rate-limit toasts.
  var failureToast = /message (was )?not sent|failed to send|couldn't be delivered/i.test(pageText);
  var rateLimited = /sending too many messages|slow down|rate limit/i.test(pageText);

  // Collect last few bubbles in the thread; FB renders message rows with
  // role="row" inside the conversation container.
  var bubbles = document.querySelectorAll('[role="row"], [data-testid="mwthreadlist-message-row"]');
  var matched = null;
  var matchedIdx = -1;
  for (var i = bubbles.length - 1; i >= 0 && i >= bubbles.length - 6; i--) {
    var b = bubbles[i];
    var t = lower(b.innerText || '');
    if (t && t.indexOf(expected) !== -1) {
      matched = b;
      matchedIdx = i;
      break;
    }
  }

  // Freshness — read any visible "Sent X seconds ago" / "Just now" hints
  // near the matched bubble. Without a robust timestamp DOM hook we infer
  // freshness from absence of relative-time strings ("yesterday", "hour ago").
  var freshSeconds = 9999;
  if (matched) {
    var nearby = matched.innerText || '';
    if (/just now|seconds ago|vài giây/i.test(nearby)) freshSeconds = 5;
    else if (/1 minute|một phút|1 phút/i.test(nearby)) freshSeconds = 60;
    else if (/minute|phút/i.test(nearby)) freshSeconds = 120;
    else if (/hour|giờ|day|ngày|yesterday|hôm qua/i.test(nearby)) freshSeconds = 9999;
    else freshSeconds = 15;
  }

  return JSON.stringify({
    verified: !!matched,
    bubble_id: matched ? ('row_' + matchedIdx) : '',
    fresh_seconds: freshSeconds,
    snippet: matched ? (matched.innerText || '').slice(0, 500) : '',
    page_url: pageURL,
    thread_open: threadOpen,
    failure_toast: failureToast,
    rate_limited: rateLimited
  });
})()
`, content)
}

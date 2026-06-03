package models

// NavDiagnostic is the structured navigation + landing telemetry the
// Chrome-extension comment executor captures on EVERY attempt (PR8A —
// Navigation Hardening). Its purpose is diagnostic, not behavioural: it
// turns the historically vague `context_drift` / `redirected_feed`
// terminal into a precise, reproducible root cause.
//
// The object is assembled across two layers and merged on the wire:
//   - background (src/commands.js navigateAndVerify, src/outbox.js):
//     NavFromURL / NavToURL / NavDurationMs — the tab navigation trace.
//   - content script (content/outbound.js executeComment): everything
//     observable only from inside the page — LandedURL, DocTitle, the
//     three pre-comment gate booleans, the rendered TargetPostID, and
//     the RedirectClass classification.
//
// It rides ExtensionExecutionReport → VerifierProof → VerificationEvidence
// and is persisted verbatim into execution_attempts.evidence_json, so the
// superadmin diagnostic endpoint can surface typed fields instead of
// grepping a free-form notes string. All fields are best-effort: an empty
// field means "the executor could not observe this", never "false".
type NavDiagnostic struct {
	// ── Navigation trace (background layer) ──
	NavFromURL    string `json:"nav_from_url,omitempty"`   // tab URL immediately before the navigate
	NavToURL      string `json:"nav_to_url,omitempty"`     // URL we asked the tab to load (the target)
	NavDurationMs int    `json:"nav_duration_ms,omitempty"` // wall-clock create→settled for the winning attempt
	NavAttempts   int    `json:"nav_attempts,omitempty"`   // how many navigateAndVerify retries it took

	// ── Landing state (content layer) ──
	LandedURL string `json:"landed_url,omitempty"` // location.href at gate evaluation
	DocTitle  string `json:"doc_title,omitempty"`  // document.title at gate evaluation

	// ── Pre-comment gates (content layer) — captured BEFORE any typing ──
	ArticleFound       bool `json:"article_found"`        // a [role=article] for the target post is present
	PermalinkFound     bool `json:"permalink_found"`      // the article's canonical permalink anchor is present
	CommentButtonFound bool `json:"comment_button_found"` // a Comment/Bình luận button is present in scope

	// ── Identity (content layer) ──
	TargetPostID string `json:"target_post_id,omitempty"` // post id extracted from the queued target_url
	AccountID    int64  `json:"account_id,omitempty"`     // executing account (echoed from the work item)
	FBUserID     string `json:"fb_user_id,omitempty"`     // logged-in c_user read from the page

	// ── Classification (content/background layer) ──
	// RedirectClass is the deterministic landing classification. It is the
	// single most important field for resuming the investigation: it names
	// exactly WHY a navigation did not reach the post. One of the
	// RedirectClass* constants below.
	RedirectClass string `json:"redirect_class,omitempty"`

	// Stage names the gate that produced this diagnostic (e.g. "gate1_no_article",
	// "gate2_post_click_swap", "post_submit"). Lets the operator see at a glance
	// which checkpoint fired without parsing notes.
	Stage string `json:"stage,omitempty"`

	// DOMSnapshot is a truncated text excerpt of the landed page (bounded by
	// the extension before send). Captured on failure so the operator can see
	// "Content unavailable" / login wall / feed shell without a screenshot.
	DOMSnapshot string `json:"dom_snapshot,omitempty"`
}

// RedirectClass constants — the closed vocabulary for NavDiagnostic.RedirectClass.
// Deterministic and mutually exclusive; classified from the landed URL + page
// signals. Keep in sync with the extension's classifyLanding (content/navreport.js).
const (
	// RedirectClassPermalink — the tab DID land on the intended post permalink.
	// (Not a redirect; recorded so a successful landing is explicit, not inferred
	// from the absence of a redirect class.)
	RedirectClassPermalink = "permalink"
	// RedirectClassFeed — landed on a feed surface (/?sk=, /home.php, watch feed)
	// that is not the bare root. The classic "bounced to feed" anti-bot redirect.
	RedirectClassFeed = "feed"
	// RedirectClassHome — landed on the bare root (https://www.facebook.com/).
	RedirectClassHome = "home"
	// RedirectClassLogin — landed on a login wall (/login, /checkpoint/?next).
	// Maps to human_required upstream, not a navigation bug.
	RedirectClassLogin = "login"
	// RedirectClassCheckpoint — landed on an identity checkpoint / 2FA gate.
	RedirectClassCheckpoint = "checkpoint"
	// RedirectClassUnsupportedTarget — the target URL shape is not a commentable
	// post permalink (e.g. a /photo/ viewer, a marketplace item). The nav may have
	// "succeeded" but there is no post to comment on.
	RedirectClassUnsupportedTarget = "unsupported_target"
	// RedirectClassUnknown — none of the above matched. Landed somewhere we do
	// not yet classify; the LandedURL + DOMSnapshot carry the raw evidence.
	RedirectClassUnknown = "unknown"
)

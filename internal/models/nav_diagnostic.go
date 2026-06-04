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
	// LandedURL is the URL the BACKGROUND navigateAndVerify confirmed the tab
	// reached (it only returns on a match, so this is normally == NavToURL).
	// FinalURL is location.href the moment the CONTENT script evaluated the
	// gate — captured separately so a late top-level redirect shows up as a
	// LandedURL(target) != FinalURL(feed/home) delta in a single row, which is
	// the PR8 "Redirect Failure" signature (landed_url != target_url).
	LandedURL string `json:"landed_url,omitempty"` // background-verified landing (≈ target)
	FinalURL  string `json:"final_url,omitempty"`  // location.href at gate evaluation (post-drift)
	DocTitle  string `json:"doc_title,omitempty"`  // document.title at gate evaluation (the page_title)

	// ── Pre-comment gates (content layer) — captured BEFORE any typing ──
	ArticleFound       bool `json:"article_found"`        // a [role=article] for the target post is present
	PermalinkFound     bool `json:"permalink_found"`      // the article's canonical permalink anchor is present
	CommentButtonFound bool `json:"comment_button_found"` // a Comment/Bình luận button is present in scope

	// ── DOM census (content layer) — PR8A evidence pack ──
	// Raw element counts on the landed page, captured at the failing gate.
	// Counts (not just the gate booleans) let the ROOT_CAUSE_REPORT separate
	// "redirect: nothing on page" (all zero) from "gate: article present but
	// composer never opened" (article_count>0, composer_count==0) etc. without
	// a screenshot. All best-effort; absent/zero means "not observed / none".
	ArticleCount         int `json:"article_count,omitempty"`         // [role=article] containers on the page
	CommentButtonCount   int `json:"comment_button_count,omitempty"`  // visible Comment/Bình luận buttons
	ComposerCount        int `json:"composer_count,omitempty"`        // contenteditable[role=textbox] comment composers
	TextareaCount        int `json:"textarea_count,omitempty"`        // <textarea> elements
	ContenteditableCount int `json:"contenteditable_count,omitempty"` // [contenteditable=true] elements

	// ── Execution phase (content layer) — PR8A proof integrity ──
	// Phase is the LAST execution phase actually REACHED before the failure,
	// one of the ExecPhase* constants. It is the deterministic boundary that
	// the classifier must respect: a failure whose Phase is < submit MUST NOT
	// be described as "after submit" / "submit failed" / "verification failed".
	// See content/proof.js buildCommentProof (proof-integrity fix).
	Phase string `json:"phase,omitempty"`

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

	// ScreenshotPath is the SERVER-written relative path of the JPEG the
	// background captured at the failing moment (data/evidence/<org>/<...>.jpg).
	// The extension ships the raw image out-of-band (ExtensionExecutionReport.
	// EvidenceScreenshotB64); the server decodes it to disk and records ONLY
	// the path here — the bytes are never stored in evidence_json. Empty when
	// capture was unavailable or the attempt succeeded.
	ScreenshotPath string `json:"screenshot_path,omitempty"`

	// Forensics is the PR8C content-script interaction recorder output: the
	// timestamped micro-timeline of every DOM op our content script performed in
	// the comment window, correlated against the MAIN-world history.pushState
	// reset (with FB's stack trace). It answers "what was the LAST operation our
	// automation did before Facebook bounced the tab to home" — the question that
	// must be settled before any technology change. Nil on legacy builds and for
	// non-comment paths. See content/forensics.js.
	Forensics *NavForensics `json:"forensics,omitempty"`

	// NavEvents is the chrome.webNavigation trace for the comment tab between
	// tab-open and the gate evaluation (PR8A.1). It NAMES the source of the
	// home-redirect: a server_redirect/client_redirect qualifier means FB
	// redirected; a 'history' kind (onHistoryStateUpdated) means FB's SPA
	// router reset; a 'typed'/'auto_toplevel' committed nav with no redirect
	// qualifier means our own chrome.tabs code moved the tab. This is the field
	// that settles "FB vs our system" with ground truth instead of inference.
	NavEvents []NavEvent `json:"nav_events,omitempty"`
}

// NavEvent is one top-frame navigation observed on the comment tab via
// chrome.webNavigation (onCommitted / onHistoryStateUpdated). See NavDiagnostic.NavEvents.
type NavEvent struct {
	URL        string `json:"url,omitempty"`
	Transition string `json:"transition,omitempty"` // webNavigation transitionType (link/typed/auto_toplevel/reload/...)
	Qualifiers string `json:"qualifiers,omitempty"` // comma-joined transitionQualifiers (server_redirect/client_redirect/forward_back/from_address_bar)
	Kind       string `json:"kind,omitempty"`       // "committed" (full nav) | "history" (SPA pushState/replaceState)
	TMs        int    `json:"t_ms,omitempty"`       // ms since the comment tab was opened
}

// NavForensics is the PR8C-Forensics payload: the timestamped record of the
// content script's own DOM interaction in the comment window, correlated with
// the Facebook home-reset pushState. All times are ms relative to EntryTS
// (the moment executeComment armed the recorder). Assembled by
// content/forensics.js; persisted verbatim into evidence_json.
type NavForensics struct {
	// EntryTS is the wall-clock (Date.now) when the recorder armed — the
	// content-script handoff moment. All TMs values below are relative to it.
	EntryTS int64 `json:"entry_ts,omitempty"`
	// Counts is op → exact total over the window (querySelectorAll, querySelector,
	// click, focus, dispatchEvent, MutationObserver.observe, innerHTML.get,
	// innerText.get) plus innerHTML_bytes / innerText_bytes totals. This directly
	// answers "how many DOM scans / clicks / focuses / how many KB of innerHTML".
	Counts map[string]int `json:"counts,omitempty"`
	// Actions is the MUTATING-op timeline (click / focus / dispatchEvent /
	// MutationObserver.observe) kept in its own buffer so the few interesting
	// interactions are never flooded out by the high-volume read ops in Timeline.
	// This is where the click that opens a picker / navigates shows up.
	Actions []ForensicEvent `json:"actions,omitempty"`
	// Timeline is the last ~50 READ micro-ops (querySelectorAll / innerText / ...)
	// before the snapshot, each with its ms offset, op name, and a short detail.
	Timeline []ForensicEvent `json:"timeline,omitempty"`
	// PushStates is the MAIN-world history.pushState/replaceState/popstate events
	// observed in the window, each with FB's stack trace at the call site.
	PushStates []ForensicPushState `json:"push_states,omitempty"`
	// ResetTMs is ms from EntryTS to the FIRST home/feed pushState after entry
	// (0 if none was observed). ResetStack is FB's stack at that reset.
	ResetTMs   int    `json:"reset_t_ms,omitempty"`
	ResetStack string `json:"reset_stack,omitempty"`
	// LastOpBeforeReset is the final content-script op at/before the reset — the
	// prime suspect for what triggered FB's bounce. Nil if no reset was observed.
	LastOpBeforeReset *ForensicEvent `json:"last_op_before_reset,omitempty"`
}

// ForensicEvent is one timestamped content-script DOM operation.
type ForensicEvent struct {
	TMs    int    `json:"t"`                // ms since NavForensics.EntryTS
	Op     string `json:"op"`               // querySelectorAll | click | focus | dispatchEvent | innerHTML.get | ...
	Detail string `json:"detail,omitempty"` // e.g. "[role=article] →2", "div[role=button] pointerdown"
}

// ForensicPushState is one MAIN-world history mutation with FB's stack trace.
type ForensicPushState struct {
	TMs    int    `json:"t"`                // ms since NavForensics.EntryTS
	URL    string `json:"url,omitempty"`    // the URL FB navigated to
	Method string `json:"method,omitempty"` // pushState | replaceState | popstate
	Stack  string `json:"stack,omitempty"`  // truncated JS stack at the call site (names the FB module)
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

// ExecPhase constants — the closed vocabulary for NavDiagnostic.Phase. They
// are ORDERED: each phase can only be reached after the previous one. The
// classifier uses this ordering to enforce the PR8 proof-integrity rule:
// a failure can only carry a "submit"/"verify" diagnostic if Phase reached
// that far. Keep in sync with content/outbound.js executeComment phase tracker.
const (
	// ExecPhaseNavigation — the tab never arrived at the target post surface
	// (bounced to feed/home/login before an article could be located). Maps to
	// the report's "Redirect Failure" group (landed_url != target_url).
	ExecPhaseNavigation = "navigation"
	// ExecPhaseGate1 — reached the post permalink, but the target article never
	// became present+stable (identity gate 1). Maps to "Gate Failure".
	ExecPhaseGate1 = "gate1"
	// ExecPhaseComposer — the target article was located and the Comment button
	// engaged, but no editable composer materialised. Maps to "Composer Failure".
	ExecPhaseComposer = "composer"
	// ExecPhaseTyping — the composer was found and text insertion was attempted
	// but the editor never held our content. Maps to "Typing Failure".
	ExecPhaseTyping = "typing"
	// ExecPhaseSubmit — text was inserted and a submit (click/enter) was issued
	// but the composer never cleared. This is the FIRST phase at which an
	// "after submit" diagnostic is legitimate.
	ExecPhaseSubmit = "submit"
	// ExecPhaseVerify — submit cleared the composer; the post-submit DOM
	// observation (node match / count) ran. Maps to "Verification Failure".
	ExecPhaseVerify = "verify"
)

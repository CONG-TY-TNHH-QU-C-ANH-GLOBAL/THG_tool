package directpost

import (
	"strings"

	"github.com/thg/scraper/internal/fburl"
)

// Browser pre-submit verification (P1.3C / Layer C). Before the executor actually clicks
// "Send" on a Facebook comment, the backend verifies the LIVE browser context the extension
// reports against the intended target. This is the last line of defense: even if a poisoned
// lead or a wrong navigation slipped past the ingest/queue guards, a mismatch here aborts
// BEFORE the irreversible submit. URL alone is not trusted — evidence is weighed in priority
// order (post id > group > URL > author/text).

// Browser pre-submit reason codes (typed, safe to log/persist).
const (
	ReasonBrowserPostIDMissing     = "browser_post_id_missing"
	ReasonBrowserPostIDMismatch    = "browser_post_id_mismatch"
	ReasonBrowserGroupMismatch     = "browser_group_mismatch"
	ReasonBrowserContextMismatch   = "browser_context_mismatch"
	ReasonBrowserContentUnverified = "browser_content_unverified"
)

// BrowserContext is what the executor observes in the live tab just before submitting.
// Every field is best-effort; the verifier uses the strongest evidence present and FAILS
// CLOSED when no usable identity evidence exists at all.
type BrowserContext struct {
	CurrentPostFBID string // visible post id / story_fbid in the open composer's post
	CurrentGroupRef string // visible group id/ref (numeric or vanity)
	CurrentURL      string // the tab's current URL
	AuthorSnippet   string // visible post author/text snippet (weakest evidence)
}

// VerifyBrowserContext checks the live browser context against the intended target and
// returns (ok, reason). It mirrors the ingest invariants but on browser-reported evidence:
//
//  1. POST ID (strongest): if the target post id is known, the live post id must be present
//     and equal it — a missing live id is browser_post_id_missing, a different id is
//     browser_post_id_mismatch. When present and equal, the target is positively confirmed.
//  2. GROUP: a live group ref that is a DIFFERENT NAMED group than expected is a mismatch.
//  3. URL: when no live post id is available, fall back to the current URL's post id /
//     canonical equivalence; if that cannot confirm the target → browser_context_mismatch.
//
// AuthorSnippet is advisory only (never the sole basis to allow OR block) — it is logged for
// debugging, not used as proof here, to avoid false aborts on cosmetic text differences.
func VerifyBrowserContext(target ExpectedTarget, live BrowserContext) (bool, string) {
	expPID := strings.TrimSpace(target.PostFBID)
	livePID := strings.TrimSpace(live.CurrentPostFBID)
	if livePID == "" {
		livePID = fburl.ExtractFacebookPostID(live.CurrentURL)
	}

	// Group mismatch is a hard fail regardless of id (a different named group is a different
	// place to post). Numeric-different is ambiguous (vanity→numeric) — handled like ingest.
	if conflict, _ := ContextConflict(target.GroupRef, live.CurrentURL, "", live.CurrentGroupRef); conflict {
		return false, ReasonBrowserGroupMismatch
	}

	if expPID != "" {
		if livePID == "" {
			return false, ReasonBrowserPostIDMissing
		}
		if livePID != expPID {
			return false, ReasonBrowserPostIDMismatch
		}
		return true, "" // post id positively confirmed
	}

	// Target post id unknown → require positive URL/canonical proof of the live tab.
	expCanon := strings.TrimSpace(target.CanonicalURL)
	if expCanon != "" {
		if canon, ok := fburl.CanonicalizePostURL(strings.TrimSpace(live.CurrentURL)); ok && canon == expCanon {
			return true, ""
		}
	}
	return false, ReasonBrowserContextMismatch
}

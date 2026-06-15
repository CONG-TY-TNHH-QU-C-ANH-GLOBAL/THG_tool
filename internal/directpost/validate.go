// Package directpost holds the ZERO-TRUST validation invariants for explicit
// direct-post comment intake. The backend must never assume a connector observation
// (the scraped item) actually is the post the user requested: a fail-open assumption
// let a Backend-Jobs post be stamped with a ship.viet.my URL and force-created as a
// lead (incident P1.3). These pure helpers are shared by the ingest path (force-lead
// gate) and the poller (pre-comment gate) so both enforce identical invariants.
//
// Layered invariants (all must hold before a direct-post item/lead is trusted):
//  1. IDENTITY is positive, never assumed — the observed post id must equal the
//     requested post id (or the source URL must positively canonicalize to the
//     requested canonical when the requested id is unknown).
//  2. CONTEXT must not conflict — a group/page author or source tied to a DIFFERENT
//     group than the requested one is a wrong-post signal.
//  3. CONTENT must be meaningful — empty / UI-chrome / boilerplate is not a post.
//
// ForceLead may bypass MARKET-FIT vetoes (cold/relevance) for an explicit request, but
// it must NOT bypass these extraction/identity/context invariants.
package directpost

import (
	"strings"

	"github.com/thg/scraper/internal/fburl"
)

// minMeaningfulRunes is the floor of real post text (after UI-chrome stripping) below
// which an extraction is treated as garbage rather than a post. Deliberately LOW: the
// chrome wordlist + duplicate-collapse below do the discrimination; the floor only rejects
// near-empty extractions, so a short-but-real post (e.g. "ai làm fulfill US") still passes.
const minMeaningfulRunes = 12

// uiChromeTokens are standalone Facebook UI words a scraper commonly captures around the
// real post text. Dropped entirely (not just collapsed) so "Like Like Comment Share" and
// "Facebook Facebook…" reduce to nothing, while real posts keep their substance.
var uiChromeTokens = map[string]bool{
	"facebook": true, "like": true, "comment": true, "comments": true,
	"share": true, "shares": true, "follow": true, "following": true,
	"reply": true, "replies": true, "reactions": true, "react": true,
}

// Reason codes (typed, safe to log/persist). Identity reasons mean "not the requested
// post"; context/content reasons mean "the requested post came back poisoned".
const (
	ReasonPostIDUnverified = "post_id_unverified"
	ReasonPostIDMismatch   = "post_id_mismatch"
	ReasonGroupConflict    = "group_context_conflict"
	ReasonContentInvalid   = "lead_content_invalid"
)

// ExpectedTarget is the requested post identity carried by the workflow.
type ExpectedTarget struct {
	PostFBID     string
	GroupRef     string
	CanonicalURL string
}

// ObservedItem is what the connector/lead actually carries. AuthorProfileURL is the
// strongest wrong-group signal: a real post author is a USER, so an author profile that
// is a /groups/{other}/ URL means the extraction grabbed a foreign-group context.
type ObservedItem struct {
	PostFBID         string
	SourceURL        string
	GroupFBID        string
	AuthorName       string
	AuthorProfileURL string
	Content          string
}

// Validation is the layered verdict. IdentityMatched distinguishes "a different/neighbour
// post" (IdentityMatched=false → skip, do not fail the workflow) from "the requested post
// but poisoned" (IdentityMatched=true, Valid=false → fail the workflow with Reason).
type Validation struct {
	IdentityMatched bool
	Valid           bool
	Reason          string
}

// Validate runs the three invariants in order: identity, then context, then content.
func Validate(exp ExpectedTarget, obs ObservedItem) Validation {
	if ok, reason := PositivePostIDMatch(exp, obs); !ok {
		return Validation{IdentityMatched: false, Valid: false, Reason: reason}
	}
	if conflict, reason := ContextConflict(exp.GroupRef, obs.SourceURL, obs.AuthorProfileURL, obs.GroupFBID); conflict {
		return Validation{IdentityMatched: true, Valid: false, Reason: reason}
	}
	if !ValidContent(obs.Content) {
		return Validation{IdentityMatched: true, Valid: false, Reason: ReasonContentInvalid}
	}
	return Validation{IdentityMatched: true, Valid: true}
}

// PositivePostIDMatch is fail-CLOSED: it returns true ONLY when the observed post id is
// present and equals the requested id (or, when the requested id is unknown, the source
// URL positively canonicalizes to the requested canonical). An absent observed id is
// NEVER assumed to be the requested post.
func PositivePostIDMatch(exp ExpectedTarget, obs ObservedItem) (bool, string) {
	obsPID := strings.TrimSpace(obs.PostFBID)
	if obsPID == "" {
		obsPID = fburl.ExtractFacebookPostID(obs.SourceURL)
	}
	expPID := strings.TrimSpace(exp.PostFBID)
	if expPID != "" {
		if obsPID == "" {
			return false, ReasonPostIDUnverified
		}
		if obsPID != expPID {
			return false, ReasonPostIDMismatch
		}
		return true, ""
	}
	// Requested id unknown → require positive canonical-URL identity, never assume.
	expCanon := strings.TrimSpace(exp.CanonicalURL)
	if expCanon != "" {
		if canon, ok := fburl.CanonicalizePostURL(strings.TrimSpace(obs.SourceURL)); ok && canon == expCanon {
			return true, ""
		}
	}
	return false, ReasonPostIDUnverified
}

// ContextConflict reports whether the observed group/source context clearly points to a
// DIFFERENT group than the requested one. Rules:
//   - AuthorProfileURL that is a /groups/{ref}/ URL with a different ref → conflict
//     (a legitimate post author is a user, so a group author = foreign-context grab;
//     any different ref counts, named or numeric).
//   - SourceURL / GroupFBID that name a DIFFERENT, NAMED (non-numeric) group → conflict.
//     A different NUMERIC group is left ambiguous (it may be a vanity→numeric redirect of
//     the same post — identity already matched), preserving valid P1.2 behavior.
//
// A normal user-profile author never trips this (ExtractGroupRef == "").
func ContextConflict(expectedGroupRef, sourceURL, authorProfileURL, groupFBID string) (bool, string) {
	exp := strings.TrimSpace(expectedGroupRef)
	if exp == "" {
		return false, ""
	}
	if g := fburl.ExtractGroupRef(authorProfileURL); g != "" && g != exp {
		return true, ReasonGroupConflict // author tied to a foreign group
	}
	if differentNamedGroup(fburl.ExtractGroupRef(sourceURL), exp) {
		return true, ReasonGroupConflict
	}
	if differentNamedGroup(strings.TrimSpace(groupFBID), exp) {
		return true, ReasonGroupConflict
	}
	return false, ""
}

// differentNamedGroup reports a clear conflict: observed is a NON-numeric group ref that
// differs from expected. Numeric observed refs are treated as ambiguous (not a conflict).
func differentNamedGroup(observed, expected string) bool {
	observed = strings.TrimSpace(observed)
	return observed != "" && expected != "" && observed != expected && !isAllDigits(observed)
}

// ValidContent reports whether content has enough meaningful post text after stripping
// Facebook UI chrome — empty / "Facebook Facebook…" / collapsed-chrome extractions fail.
func ValidContent(content string) bool {
	return len([]rune(strings.TrimSpace(MeaningfulText(content)))) >= minMeaningfulRunes
}

// MeaningfulText strips standalone "Facebook" UI tokens and collapses runs of identical
// consecutive tokens (the repetition signature of scraped chrome), returning the
// remaining real text. Pure and order-preserving.
func MeaningfulText(content string) string {
	var out []string
	prev := ""
	for _, f := range strings.Fields(content) {
		norm := strings.ToLower(strings.Trim(f, "·.,:;!?()[]{}\"'…"))
		if norm == "" || uiChromeTokens[norm] {
			continue // drop FB UI chrome tokens (Facebook, Like, Comment, Share…)
		}
		if norm == prev {
			continue // collapse repeated tokens (the scraped-chrome repetition signature)
		}
		out = append(out, f)
		prev = norm
	}
	return strings.Join(out, " ")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

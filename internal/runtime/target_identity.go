package runtime

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/fburl"
	"github.com/thg/scraper/internal/models"
)

// EnforceTargetIdentity is the backend defense-in-depth check that
// guarantees a successful outbound comment landed on the SPECIFIC
// entity that was queued, NOT just "any" entity where a comment node
// happened to match the queued content. It is the load-bearing
// invariant that prevents the May-2026 incident (comment routed to a
// volleyball post when the target was a dropship group post — extension
// reported success, server accepted it) from recurring.
//
// # Threat model
//
// The Chrome extension can be fooled when Facebook's SPA renders the
// wrong post above the target during the post-navigation race
// window: the extension finds a comment-able surface, posts the
// content there, sees a matching author + text node, and reports
// success. Without this guard the server would accept the report and
// mark the outbound `sent` on the wrong post.
//
// We do not trust the extension as sole authority. The extension's
// observation must be independently corroborated by a server-side
// id match between the queued target_url and the proof's
// page_url_after.
//
// # Contract (rules from the change request)
//
// Verification success requires BOTH:
//
//  1. the executor produced a success-class outcome (comment exists);
//     this function leaves failure-class outcomes alone — they are
//     already correctly classified by the upstream verifier.
//  2. the canonical entity id extracted from target_url MATCHES the
//     canonical entity id extracted from proof.PageURLAfter.
//
// When the match fails, the outcome is forcibly downgraded to
// [models.ExecutionContextDrift] regardless of what the executor
// reported. The proof.Notes field is annotated with a structured
// "entity-drift target=… actual=…" line so the operator-replay UI
// can show WHY we overrode the executor's verdict.
//
// # Fail-closed cases
//
//  1. Empty actionType OR actionType != "comment": pass through
//     unchanged. Inbox / post identity checks have different recipes
//     (thread id vs feed scope) and are wired in their own guards.
//     Defaulting to "no check" here would silently expand this
//     function's responsibility surface as new action types land.
//
//  2. targetURL empty or unparseable (no recognisable entity id):
//     fail closed — downgrade success-class outcomes to ContextDrift.
//     If we can't independently verify the destination from the
//     queued URL, we refuse to trust the extension's claim.
//
//  3. proof.PageURLAfter empty or unparseable: fail closed — same
//     reasoning. An extension that omits page_url_after has not given
//     us enough evidence to corroborate.
//
// # What this function does NOT do
//
//   - It does NOT re-verify content match (that's the upstream
//     verifier's job).
//   - It does NOT add new success outcomes; it only DOWNGRADES.
//   - It does NOT mutate the outcome when the action is non-comment.
//     Future PRs may add EnforceInboxIdentity / EnforcePostIdentity
//     with their own recipes.
//   - It does NOT call the database or any network — pure function,
//     fully unit-testable.
func EnforceTargetIdentity(
	outcome models.ExecutionOutcome,
	proof VerifierProof,
	targetURL string,
	actionType string,
) (models.ExecutionOutcome, VerifierProof) {
	// Only comment actions enforce post identity in this guard. Inbox
	// and post types have different identity semantics.
	if strings.ToLower(strings.TrimSpace(actionType)) != "comment" {
		return outcome, proof
	}
	// Failure outcomes are already correctly classified upstream — a
	// rate_limited or blocked action does not become more accurate by
	// being re-labelled context_drift, and the risk-signal pipeline
	// would lose its specific failure signal.
	if !models.IsSuccessOutcome(outcome) {
		return outcome, proof
	}

	target := fburl.ExtractFacebookEntityID(strings.TrimSpace(targetURL))
	actual := fburl.ExtractFacebookEntityID(strings.TrimSpace(proof.PageURLAfter))

	// SameFacebookEntity returns false when either side is empty (fail
	// closed by construction — see fburl tests). We re-inline the
	// extraction here so we can annotate the proof with the specific
	// ids that failed to match, which is the information operator
	// replay needs to debug the drift.
	if target == "" || actual == "" || target != actual {
		annotation := fmt.Sprintf(
			"entity-drift target=%q actual=%q",
			abbreviateID(target),
			abbreviateID(actual),
		)
		if proof.Notes == "" {
			proof.Notes = annotation
		} else {
			proof.Notes = proof.Notes + " · " + annotation
		}
		return models.ExecutionContextDrift, proof
	}
	return outcome, proof
}

// abbreviateID keeps the operator-replay annotation short. pfbid
// tokens run >60 chars; surfacing the first 16 is enough to tell two
// entities apart without bloating the evidence_json column.
func abbreviateID(id string) string {
	if id == "" {
		return "<missing>"
	}
	if len(id) <= 16 {
		return id
	}
	return id[:16] + "…"
}

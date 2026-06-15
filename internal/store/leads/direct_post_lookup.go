package leads

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/fburl"
	"github.com/thg/scraper/internal/models"
)

// Strict post-lead matching for the direct-post intake flow (hotfix
// fix/direct-post-intake-identity-mismatch). A bare post_fbid match is unsafe: a
// Facebook GROUP permalink id and a global story_fbid can be DIFFERENT posts sharing
// the same number — which let a generic permalink.php lead attach to a group post.

// directPostGroupCompatible reports whether a candidate post lead's source URL is a
// PROVABLY-SAFE match for a direct-post workflow's group context. The bar is exact
// group identity only:
//
//   - the SAME group ref (vanity==vanity or numeric==numeric) → compatible;
//   - generic permalink.php?story_fbid= (no group context) → NOT compatible (the
//     production bug: a Data-Engineer permalink.php lead matched a ship.viet.my post);
//   - a DIFFERENT group ref (named OR numeric) → NOT compatible.
//
// We do NOT accept a numeric group merely because the request was a vanity slug: a
// Facebook vanity→numeric redirect of the SAME post and an UNRELATED numeric group
// that happens to share the post id are indistinguishable WITHOUT import provenance
// (the canonical `leads` table carries no import_task linkage). Accepting it blind
// would reintroduce the cross-context wrong-attach class this hotfix exists to close.
// A legit redirect therefore matches only via the exact-canonical path in
// GetPostLeadByDirectPostRef (the import navigates to the canonical URL); when that
// fails it degrades safely to retry/lead_not_observed, never a wrong comment. Proper
// vanity→numeric acceptance needs a lead↔import-task provenance link (follow-up).
//
// groupRef=="" (a non-group direct post) never matches via this path — only the exact
// canonical URL may match a non-group post — so a group lead can never be attached to
// a non-group request by a bare post_fbid.
func directPostGroupCompatible(leadSourceURL, groupRef string) bool {
	if groupRef == "" || !fburl.IsGroupPermalinkURL(leadSourceURL) {
		return false
	}
	return fburl.ExtractGroupRef(leadSourceURL) == groupRef
}

// isDirectPostConflict reports whether a same-post_fbid lead is DEFINITELY a different
// post from the requested group post (the wrong-attach bug class) — i.e. safe to fail
// the workflow with imported_lead_identity_mismatch:
//
//   - a generic permalink.php lead (no group context — the original decoy), OR
//   - a DIFFERENT NAMED (non-numeric) group.
//
// A DIFFERENT NUMERIC group is AMBIGUOUS (it may be a vanity→numeric redirect of the
// same post — unprovable without import provenance), so it is NOT asserted as a
// conflict: the poller retries rather than claiming a mismatch it cannot stand behind.
func isDirectPostConflict(leadSourceURL, groupRef string) bool {
	if groupRef == "" {
		return false
	}
	if !fburl.IsGroupPermalinkURL(leadSourceURL) {
		return true
	}
	leadGroup := fburl.ExtractGroupRef(leadSourceURL)
	if leadGroup == "" || leadGroup == groupRef {
		return false
	}
	return !isAllDigits(leadGroup)
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

// postLeadsByFBID returns the org-scoped, non-archived POST leads with this post_fbid
// (most recent first). The strict matchers compare group context in Go (SQL LIKE can't
// express the numeric-group tolerance safely).
func (s *Store) postLeadsByFBID(ctx context.Context, orgID int64, postFBID string) ([]models.Lead, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+leadLookupColumns+` FROM leads l
		 WHERE l.org_id = ? AND l.archived_at IS NULL AND l.source_type = 'post' AND l.post_fbid = ?
		 ORDER BY l.created_at DESC, l.id DESC`, orgID, postFBID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Lead
	for rows.Next() {
		l, err := scanLeadRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetPostLeadByDirectPostRef is the STRICT post-lead lookup for direct-post intake.
// Matches ONLY: (1) exact canonical source_url, OR (2) same post_fbid AND a
// group-compatible source (directPostGroupCompatible). It NEVER matches by bare
// post_fbid and NEVER a generic permalink.php lead. Org-scoped, post-only, archived
// excluded. (nil, nil) when no SAFE match exists.
func (s *Store) GetPostLeadByDirectPostRef(ctx context.Context, orgID int64, postFBID, canonicalURL, groupRef string) (*models.Lead, error) {
	if strings.TrimSpace(canonicalURL) != "" {
		row := s.db.QueryRowContext(ctx,
			`SELECT `+leadLookupColumns+` FROM leads l
			 WHERE l.org_id = ? AND l.archived_at IS NULL AND l.source_type = 'post' AND l.source_url = ?
			 ORDER BY l.created_at DESC, l.id DESC LIMIT 1`, orgID, canonicalURL)
		if l, err := oneLeadOrNil(row); err != nil || l != nil {
			return l, err
		}
	}
	if strings.TrimSpace(postFBID) == "" {
		return nil, nil
	}
	cands, err := s.postLeadsByFBID(ctx, orgID, postFBID)
	if err != nil {
		return nil, err
	}
	for i := range cands {
		if directPostGroupCompatible(cands[i].SourceURL, groupRef) {
			return &cands[i], nil
		}
	}
	return nil, nil
}

// FindConflictingPostLead returns a post lead that shares the requested post_fbid but
// is DEFINITELY a different post (isDirectPostConflict: a generic permalink.php lead or
// a different NAMED group) — the "decoy" a bare post_fbid match would have wrongly
// attached. The poller uses it to fail a workflow with imported_lead_identity_mismatch
// instead of attaching the wrong lead. An AMBIGUOUS numeric-group lead is deliberately
// NOT returned here (it is not a provable conflict) so the poller retries rather than
// asserting a mismatch. Only meaningful for group posts (groupRef != ""); (nil, nil)
// otherwise.
func (s *Store) FindConflictingPostLead(ctx context.Context, orgID int64, postFBID, canonicalURL, groupRef string) (*models.Lead, error) {
	if strings.TrimSpace(postFBID) == "" || strings.TrimSpace(groupRef) == "" {
		return nil, nil
	}
	cands, err := s.postLeadsByFBID(ctx, orgID, postFBID)
	if err != nil {
		return nil, err
	}
	for i := range cands {
		if cands[i].SourceURL == canonicalURL {
			continue // the exact requested post is not a conflict
		}
		if isDirectPostConflict(cands[i].SourceURL, groupRef) {
			return &cands[i], nil
		}
	}
	return nil, nil
}

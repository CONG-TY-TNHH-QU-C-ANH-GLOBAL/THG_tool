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
// SAFE match for a direct-post workflow's group context.
//
//   - generic permalink.php?story_fbid= (no group context) → NOT compatible (the
//     production bug: a Data-Engineer permalink.php lead matched a ship.viet.my post);
//   - same group ref (vanity==vanity or numeric==numeric) → compatible;
//   - requested vanity + observed NUMERIC group → compatible (Facebook vanity→numeric
//     redirect of the SAME post the import navigated to);
//   - a DIFFERENT named group → NOT compatible.
//
// When the workflow carries no group context (groupRef==""), any group permalink is
// accepted best-effort (still rejects the lossy permalink.php form).
func directPostGroupCompatible(leadSourceURL, groupRef string) bool {
	if !fburl.IsGroupPermalinkURL(leadSourceURL) {
		return false
	}
	leadGroup := fburl.ExtractGroupRef(leadSourceURL)
	if leadGroup == "" {
		return false
	}
	if groupRef == "" || leadGroup == groupRef {
		return true
	}
	return isAllDigits(leadGroup) // vanity→numeric redirect of the same post
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
// is NOT group-compatible (a different group, or a generic permalink.php lead) — the
// "decoy" a bare post_fbid match would have wrongly attached. The poller uses it to
// fail a workflow with imported_lead_identity_mismatch instead of attaching the wrong
// lead. Only meaningful for group posts (groupRef != ""); (nil, nil) otherwise.
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
		if !directPostGroupCompatible(cands[i].SourceURL, groupRef) {
			return &cands[i], nil
		}
	}
	return nil, nil
}

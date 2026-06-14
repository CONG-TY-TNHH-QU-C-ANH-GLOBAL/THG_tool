package leads

import (
	"context"
	"database/sql"

	"github.com/thg/scraper/internal/models"
)

// leadLookupColumns mirrors the GetLeadsFiltered projection so a looked-up lead
// carries the full shape the comment planner needs (id + content + URLs + post/
// group FBIDs + score). (Future DRY: extract a shared scanLead used by both.)
const leadLookupColumns = `l.id, COALESCE(l.org_id,0), l.source_type, l.source_id,
	COALESCE(NULLIF(l.source_url,''),''),
	COALESCE(l.secondary_url,''), COALESCE(l.post_fbid,''), COALESCE(l.comment_fbid,''), COALESCE(l.group_fbid,''),
	l.platform, l.author, l.author_url, l.content, l.score, l.service_match,
	l.author_role, l.pain_point, l.ai_reasoning, COALESCE(NULLIF(l.niche,''),'logistics'),
	COALESCE(NULLIF(l.thread_role,''),'intent_originator'),
	l.classified_at, l.created_at`

func scanLeadRow(row interface{ Scan(...any) error }) (models.Lead, error) {
	var l models.Lead
	err := row.Scan(&l.ID, &l.OrgID, &l.SourceType, &l.SourceID, &l.SourceURL,
		&l.SecondaryURL, &l.PostFBID, &l.CommentFBID, &l.GroupFBID, &l.Platform,
		&l.Author, &l.AuthorURL, &l.Content, &l.Score, &l.ServiceMatch,
		&l.AuthorRole, &l.PainPoint, &l.AIReasoning, &l.Niche, &l.ThreadRole,
		&l.ClassifiedAt, &l.CreatedAt)
	if err != nil {
		return l, err
	}
	repairLeadSourceURL(&l)
	return l, nil
}

// GetLeadByID returns the org-scoped, non-archived lead by primary key, or
// (nil, nil) when absent.
func (s *Store) GetLeadByID(ctx context.Context, orgID, id int64) (*models.Lead, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+leadLookupColumns+` FROM leads l
		 WHERE l.id = ? AND l.org_id = ? AND l.archived_at IS NULL`, id, orgID)
	return oneLeadOrNil(row)
}

// GetLeadByPostRef finds an org-scoped lead for a canonical Facebook post — by
// Facebook post id (stable across URL shapes) OR exact canonical source_url.
// Returns (nil, nil) when no lead exists for that post; the caller MUST NOT
// fabricate post content. Lets the direct-link comment flow reuse an existing
// lead's content + coverage instead of a synthetic shell.
func (s *Store) GetLeadByPostRef(ctx context.Context, orgID int64, postFBID, canonicalURL string) (*models.Lead, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+leadLookupColumns+` FROM leads l
		 WHERE l.org_id = ? AND l.archived_at IS NULL
		   AND ((? <> '' AND l.post_fbid = ?) OR (? <> '' AND l.source_url = ?))
		 ORDER BY l.created_at DESC LIMIT 1`,
		orgID, postFBID, postFBID, canonicalURL, canonicalURL)
	return oneLeadOrNil(row)
}

func oneLeadOrNil(row interface{ Scan(...any) error }) (*models.Lead, error) {
	l, err := scanLeadRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

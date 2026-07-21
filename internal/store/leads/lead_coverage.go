package leads

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// GetLeadCoverageState projects the multi-actor coverage picture for a lead from the
// VERIFIED engagement ledger + the conversation thread + the ACTUAL prior comment
// texts (spec: specs/domains/facebook-sales-intelligence/features/multi-actor-coverage/technical.md). website is the org's grounded
// website used to decide website_already_used. The planner uses the result to decide
// whether ANOTHER actor may cover this lead and to shape that actor's persona.
func (s *Store) GetLeadCoverageState(ctx context.Context, orgID, leadID int64, website string) (*models.LeadCoverageState, error) {
	eng, err := s.GetLeadEngagement(ctx, orgID, leadID)
	if err != nil {
		return nil, err
	}
	lead, err := s.getLeadForOrg(ctx, orgID, leadID)
	if err != nil {
		return nil, err
	}
	texts, _ := s.listVerifiedCommentTexts(ctx, orgID, engagementMatchURLs(lead)) // best-effort
	st := models.ProjectLeadCoverage(eng.Entries, s.leadReplied(orgID, lead), texts, website)
	return &st, nil
}

// leadReplied reports whether the lead has sent us an inbound reply (engagement back),
// keyed on the lead's profile URL — the StopIfLeadReplies signal. // tenant-ok
func (s *Store) leadReplied(orgID int64, lead *models.Lead) bool {
	if lead == nil {
		return false
	}
	url := strings.TrimSpace(lead.AuthorURL)
	if url == "" {
		return false
	}
	thread, err := s.Threads().GetThreadByProfileForOrg(orgID, url)
	if err != nil || thread == nil {
		return false
	}
	return !thread.LastInboundAt.IsZero()
}

// listVerifiedCommentTexts returns the content of VERIFIED comments on the lead's
// target URLs. Verified truth = action_ledger (outcome='succeeded'); content is read
// from outbound_messages via al.outbound_id. // tenant-ok cross-domain read.
func (s *Store) listVerifiedCommentTexts(ctx context.Context, orgID int64, urls []string) ([]string, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(urls))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(urls)+1)
	args = append(args, orgID)
	for _, u := range urls {
		args = append(args, u)
	}
	query := `
		SELECT COALESCE(om.content, '')
		  FROM action_ledger al
		  JOIN outbound_messages om ON om.id = al.outbound_id
		 WHERE al.org_id = ?
		   AND al.outcome = 'succeeded'
		   AND al.action_type = 'comment'
		   AND al.target_url IN (` + placeholders + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		if strings.TrimSpace(c) != "" {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

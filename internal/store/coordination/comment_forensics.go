package coordination

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// Comment Verification Forensics (spec: specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md, PR-1 Part A).
// Read-only diagnostic: for each target URL, join the latest outbound + its latest
// execution_attempt + the action_ledger outcome, parse the persisted evidence_json, and
// classify. Coordination owns the verification truth (action_ledger + execution_attempts);
// outbound_messages + accounts are cross-domain READS via raw SQL (no peer import).

// CommentForensicsByTargetURLs returns one forensic row per target URL (latest attempt).
func (s *Store) CommentForensicsByTargetURLs(ctx context.Context, orgID int64, urls []string) ([]models.CommentForensicsRow, error) {
	if orgID <= 0 || len(urls) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(urls))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(urls)+1)
	args = append(args, orgID)
	for _, u := range urls {
		args = append(args, strings.TrimSpace(u))
	}
	// tenant-ok: cross-domain read (coordination -> outbound) for the 2-column state.
	rows, err := s.db.QueryContext(ctx,
		`SELECT om.id, COALESCE(om.execution_id,''), om.target_url, om.account_id,
		        COALESCE(om.execution_state,''), COALESCE(om.verification_outcome,'')
		   FROM outbound_messages om
		  WHERE om.org_id = ? AND om.type = 'comment' AND om.target_url IN (`+placeholders+`)
		  ORDER BY om.created_at DESC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var out []models.CommentForensicsRow
	for rows.Next() {
		var r models.CommentForensicsRow
		if err := rows.Scan(&r.OutboundID, &r.ExecutionID, &r.TargetURL, &r.AccountID,
			&r.ExecutionState, &r.VerificationOutcome); err != nil {
			return nil, err
		}
		if seen[r.TargetURL] { // keep only the latest outbound per URL
			continue
		}
		seen[r.TargetURL] = true
		s.enrichForensicsRow(ctx, orgID, &r)
		r.FillDerivedForensics()
		out = append(out, r)
	}
	return out, rows.Err()
}

// enrichForensicsRow fills the attempt outcome, ledger outcome, actor display, and parses
// evidence_json for the persisted proof fields.
func (s *Store) enrichForensicsRow(ctx context.Context, orgID int64, r *models.CommentForensicsRow) {
	// Latest execution_attempt for this outbound (owned table).
	var evidenceJSON string
	// id DESC tiebreaks the queue-time stub attempt (outcome='') vs the executor's
	// terminal attempt when both share a same-second started_at.
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(outcome,''), COALESCE(failure_reason,''), COALESCE(evidence_json,'{}')
		   FROM execution_attempts
		  WHERE outbound_id = ? AND org_id = ?
		  ORDER BY started_at DESC, id DESC LIMIT 1`,
		r.OutboundID, orgID,
	).Scan(&r.AttemptOutcome, &r.FailureReason, &evidenceJSON)

	// Ledger outcome for this outbound (owned table).
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(outcome,'') FROM action_ledger
		  WHERE outbound_id = ? AND org_id = ?
		  ORDER BY performed_at DESC LIMIT 1`,
		r.OutboundID, orgID,
	).Scan(&r.LedgerOutcome)

	// Actor display. tenant-ok: cross-domain read (coordination -> identities).
	if r.AccountID > 0 {
		_ = s.db.QueryRowContext(ctx,
			`SELECT COALESCE(NULLIF(fb_display_name,''), name, '') FROM accounts WHERE id = ?`,
			r.AccountID,
		).Scan(&r.ActorDisplay)
	}

	s.enrichReverifyState(ctx, orgID, r)
	parseForensicsEvidence(evidenceJSON, r)
}

// enrichReverifyState reads the async-reverify queue row + any appended correction so the
// report can show where the pipeline stands for this comment.
func (s *Store) enrichReverifyState(ctx context.Context, orgID int64, r *models.CommentForensicsRow) {
	var (
		outcome     sql.NullString
		reason      sql.NullString
		attemptedAt sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT outcome, reason, attempted_at FROM comment_reverify
		  WHERE outbound_id = ? AND org_id = ?`,
		r.OutboundID, orgID,
	).Scan(&outcome, &reason, &attemptedAt)
	if err == nil {
		r.ReverifyScheduled = true
		r.ReverifyOutcome = outcome.String
		r.ReverifyReason = reason.String
		r.ReverifyAttemptedAt = attemptedAt.String
	}

	// A reverify correction is a 'succeeded' ledger row with reason='reverified'.
	var correctionID sql.NullInt64
	_ = s.db.QueryRowContext(ctx,
		`SELECT id FROM action_ledger
		  WHERE outbound_id = ? AND org_id = ? AND outcome = 'succeeded' AND reason = ?
		  ORDER BY performed_at DESC LIMIT 1`,
		r.OutboundID, orgID, ReverifyCorrectionReason,
	).Scan(&correctionID)
	if correctionID.Valid {
		r.CorrectionEventID = correctionID.Int64
		r.LatestEffectiveOutcome = "succeeded"
	} else {
		r.LatestEffectiveOutcome = r.LedgerOutcome
	}
}

// parseForensicsEvidence unpacks the persisted evidence_json into the forensic row.
func parseForensicsEvidence(evidenceJSON string, r *models.CommentForensicsRow) {
	var ev struct {
		CommentPermalink string                 `json:"comment_permalink"`
		PageURLAfter     string                 `json:"page_url_after"`
		ScreenshotPath   string                 `json:"screenshot_path"`
		Notes            string                 `json:"notes"`
		NavDiagnostic    *models.NavDiagnostic   `json:"nav_diagnostic"`
	}
	if strings.TrimSpace(evidenceJSON) == "" {
		return
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &ev); err != nil {
		return
	}
	r.CommentPermalink = ev.CommentPermalink
	r.PageURLAfter = ev.PageURLAfter
	r.EvidenceScreenshotPath = ev.ScreenshotPath
	r.Notes = ev.Notes
	if ev.NavDiagnostic != nil {
		r.Phase = ev.NavDiagnostic.Phase
		r.RedirectClass = ev.NavDiagnostic.RedirectClass
		r.NavDiagnosticSummary = navDiagnosticSummary(ev.NavDiagnostic)
	}
}

// navDiagnosticSummary renders the nav telemetry as a compact one-liner.
func navDiagnosticSummary(d *models.NavDiagnostic) string {
	parts := []string{}
	if d.Phase != "" {
		parts = append(parts, "phase="+d.Phase)
	}
	if d.RedirectClass != "" {
		parts = append(parts, "redirect="+d.RedirectClass)
	}
	if d.ArticleFound {
		parts = append(parts, "article_found")
	}
	if d.CommentButtonFound {
		parts = append(parts, "comment_button_found")
	}
	return strings.Join(parts, " ")
}

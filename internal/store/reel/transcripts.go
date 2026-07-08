package reel

import (
	"context"
	"database/sql"
	"fmt"
)

// reel_transcripts accessors (migration 0112). Same discipline as reels.go:
// const literal SQL, Postgres-only, org_id/reel_id as bound $N parameters.
const (
	createTranscriptSQL = `INSERT INTO reel_transcripts (org_id, reel_id, segments, lang_src, lang_tgt, source, cost_usd) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`

	getLatestTranscriptSQL = `SELECT id, org_id, reel_id, segments, lang_src, lang_tgt, source, cost_usd, created_at FROM reel_transcripts WHERE reel_id = $1 AND org_id = $2 ORDER BY id DESC LIMIT 1`
)

// CreateTranscript stores one transcript (with timing segments) for a reel
// and returns its id. The composite FK (org_id, reel_id) -> reels(org_id,
// id) rejects a reel_id that does not belong to orgID, so a cross-org write
// fails at INSERT time rather than by application convention.
func (s *Store) CreateTranscript(ctx context.Context, orgID, reelID int64, segments, langSrc, langTgt, source string, costUSD float64) (int64, error) {
	if err := s.requirePostgres(); err != nil {
		return 0, err
	}
	if orgID <= 0 || reelID <= 0 {
		return 0, fmt.Errorf("reel: org_id and reel_id are required")
	}
	return s.insertReturningID(ctx, createTranscriptSQL, orgID, reelID, segments, langSrc, langTgt, source, costUSD)
}

// GetLatestTranscript returns the most recent transcript for a reel, or
// sql.ErrNoRows if none exists or the reel belongs to a different org.
func (s *Store) GetLatestTranscript(ctx context.Context, orgID, reelID int64) (*Transcript, error) {
	if err := s.requirePostgres(); err != nil {
		return nil, err
	}
	if orgID <= 0 || reelID <= 0 {
		return nil, sql.ErrNoRows
	}
	var tr Transcript
	err := s.db.QueryRowContext(ctx, getLatestTranscriptSQL, reelID, orgID).Scan(
		&tr.ID, &tr.OrgID, &tr.ReelID, &tr.Segments, &tr.LangSrc,
		&tr.LangTgt, &tr.Source, &tr.CostUSD, &tr.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

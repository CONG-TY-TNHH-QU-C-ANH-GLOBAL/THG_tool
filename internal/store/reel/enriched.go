package reel

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Enriched-format accessors for the columns migration 0112 added to reels.
// Same rules as reels.go: every statement is a compile-time const literal,
// Postgres-only, guarded by requirePostgres; org_id/reel_id travel only as
// bound $N parameters. These are separate from the PR-R1 base Reel scan so
// adding enriched columns never widened GetReel/ListReels.
const (
	getEnrichedSQL = `SELECT reel_type, source_key, input_branch, avatar_key, final_output_key, total_cost_usd, COALESCE(render_idempotency_key, '') FROM reels WHERE id = $1 AND org_id = $2`

	setSourceSQL = `UPDATE reels SET source_key = $1, input_branch = $2, updated_at = NOW() WHERE id = $3 AND org_id = $4`

	setAvatarKeySQL = `UPDATE reels SET avatar_key = $1, updated_at = NOW() WHERE id = $2 AND org_id = $3`

	setFinalOutputSQL = `UPDATE reels SET final_output_key = $1, updated_at = NOW() WHERE id = $2 AND org_id = $3`

	addCostSQL = `UPDATE reels SET total_cost_usd = total_cost_usd + $1, updated_at = NOW() WHERE id = $2 AND org_id = $3`

	// Money invariant: claim the render slot only if no key is set yet. A
	// second call (retry/crash) matches 0 rows and returns claimed=false, so
	// no second paid render is ever started for the same reel.
	claimRenderSQL = `UPDATE reels SET render_idempotency_key = $1, render_lease_expiry = $2, updated_at = NOW() WHERE id = $3 AND org_id = $4 AND render_idempotency_key IS NULL`
)

// GetEnriched returns the enriched-format key/cost view of a reel, or
// sql.ErrNoRows if no such reel exists or it belongs to a different org.
func (s *Store) GetEnriched(ctx context.Context, orgID, reelID int64) (*Enriched, error) {
	if err := s.requirePostgres(); err != nil {
		return nil, err
	}
	if orgID <= 0 || reelID <= 0 {
		return nil, sql.ErrNoRows
	}
	var e Enriched
	err := s.db.QueryRowContext(ctx, getEnrichedSQL, reelID, orgID).Scan(
		&e.ReelType, &e.SourceKey, &e.InputBranch, &e.AvatarKey,
		&e.FinalOutputKey, &e.TotalCostUSD, &e.RenderIdempotencyKey,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// SetSource records the uploaded source clip's R2 key and the classified
// input branch ('audio'|'vision'). Org-scoped: a foreign reel_id is a
// silent no-op.
func (s *Store) SetSource(ctx context.Context, orgID, reelID int64, sourceKey, inputBranch string) error {
	return s.execEnriched(ctx, orgID, reelID, setSourceSQL, sourceKey, inputBranch, reelID, orgID)
}

// SetAvatarKey records the R2 key of the HeyGen avatar clip.
func (s *Store) SetAvatarKey(ctx context.Context, orgID, reelID int64, avatarKey string) error {
	return s.execEnriched(ctx, orgID, reelID, setAvatarKeySQL, avatarKey, reelID, orgID)
}

// SetFinalOutput records the R2 key of the Remotion-composed final video.
func (s *Store) SetFinalOutput(ctx context.Context, orgID, reelID int64, finalKey string) error {
	return s.execEnriched(ctx, orgID, reelID, setFinalOutputSQL, finalKey, reelID, orgID)
}

// AddCost accrues provider spend onto the reel's running total.
func (s *Store) AddCost(ctx context.Context, orgID, reelID int64, deltaUSD float64) error {
	return s.execEnriched(ctx, orgID, reelID, addCostSQL, deltaUSD, reelID, orgID)
}

// ClaimRender atomically claims the reel's single render slot for key,
// returning claimed=true only for the first caller. A later retry/crash
// finds the key already set and returns claimed=false — the guard that
// enforces the money invariant (never a second paid render). leaseExpiry
// marks the claim for orphan detection.
func (s *Store) ClaimRender(ctx context.Context, orgID, reelID int64, key string, leaseExpiry time.Time) (bool, error) {
	if err := s.requirePostgres(); err != nil {
		return false, err
	}
	if orgID <= 0 || reelID <= 0 || key == "" {
		return false, fmt.Errorf("reel: org_id, reel_id and idempotency key are required")
	}
	res, err := s.db.ExecContext(ctx, claimRenderSQL, key, leaseExpiry, reelID, orgID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// execEnriched is the shared UPDATE path for the setter methods above:
// requirePostgres + id validation, then run the (const) statement.
func (s *Store) execEnriched(ctx context.Context, orgID, reelID int64, query string, args ...any) error {
	if err := s.requirePostgres(); err != nil {
		return err
	}
	if orgID <= 0 || reelID <= 0 {
		return fmt.Errorf("reel: org_id and reel_id are required")
	}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/security"
)

// Workspace Knowledge OS — Layer 3 persistence.
//
// Four load-bearing invariants (per [specs/WORKSPACE_KNOWLEDGE_OS.md]):
//
//  1. Tenant isolation: every method gates on org_id.
//  2. Idempotent ingest: re-syncing updates existing assets via the
//     (org_id, source_id, external_id) UNIQUE INDEX, never duplicates.
//  3. Operator state survives re-sync: Set* and Upsert use different
//     SQL statements that touch different columns.
//  4. Retrieval reads approved-only by default: filter.States = []
//     (the empty default at the *list-API* level) maps to {approved}
//     when called from the runtime; the Product Explorer panel
//     explicitly passes {pending,approved,hidden} to see everything.
//     The repository does NOT enforce a default here — the caller's
//     filter is honored verbatim — because the retrieval-engine layer
//     is the right place to enforce "approved-only by default."

// GetKnowledgeAsset returns one asset owned by orgID, or sql.ErrNoRows.
func (s *Store) GetKnowledgeAsset(ctx context.Context, assetID, orgID int64) (*assets.Asset, error) {
	if assetID <= 0 || orgID <= 0 {
		return nil, sql.ErrNoRows
	}
	row := s.QueryRowContext(ctx, knowledgeAssetSelect+`
		WHERE id = ? AND org_id = ?`, assetID, orgID)
	asset, err := scanKnowledgeAsset(row)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	return asset, err
}

// ListKnowledgeAssetsForOrg returns assets matching the filter. Org
// isolation is implicit on the repository receiver — there is no way
// to call this method for a tenant other than orgID.
func (s *Store) ListKnowledgeAssetsForOrg(ctx context.Context, orgID int64, filter assets.ListFilter) ([]*assets.Asset, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge_assets: org_id is required")
	}
	q := knowledgeAssetSelect + ` WHERE org_id = ?`
	args := []any{orgID}

	if len(filter.Types) > 0 {
		q += ` AND type IN (` + placeholders(len(filter.Types)) + `)`
		for _, t := range filter.Types {
			args = append(args, string(t))
		}
	}
	if len(filter.States) > 0 {
		q += ` AND state IN (` + placeholders(len(filter.States)) + `)`
		for _, s := range filter.States {
			args = append(args, string(s))
		}
	}
	if filter.SourceID > 0 {
		q += ` AND source_id = ?`
		args = append(args, filter.SourceID)
	}
	if strings.TrimSpace(filter.SearchQ) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(filter.SearchQ)) + "%"
		q += ` AND (LOWER(title) LIKE ? OR LOWER(tags) LIKE ? OR LOWER(description) LIKE ?)`
		args = append(args, like, like, like)
	}
	switch filter.OrderBy {
	case assets.OrderRecent:
		q += ` ORDER BY updated_at DESC`
	default:
		// OrderDefault matches idx_knowledge_assets_org_pin_boost so
		// the hot path is index-only on SQLite.
		q += ` ORDER BY pinned DESC, boost DESC, retrieval_count_30d DESC`
	}
	if filter.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, filter.Limit)
		if filter.Offset > 0 {
			q += ` OFFSET ?`
			args = append(args, filter.Offset)
		}
	}

	rows, err := s.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*assets.Asset, 0, 16)
	for rows.Next() {
		a, err := scanKnowledgeAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpsertKnowledgeAsset is the ingestor-side write. It writes the
// ingestor-controlled columns (type, title, description, tags,
// payload, external_id) and intentionally does NOT touch the
// operator-controlled columns (state, pinned, boost) or the
// system-controlled columns (metrics, retrieval counters).
//
// Idempotency: when (org_id, source_id, external_id) already exists,
// it does an UPDATE. The uq_knowledge_assets_idem partial index makes
// this an atomic ON CONFLICT path.
//
// Assets without a stable external_id (empty string) are excluded
// from the unique index — the ingestor must compute and pass
// [assets.ContentFingerprint] in that case to get idempotency.
func (s *Store) UpsertKnowledgeAsset(ctx context.Context, a *assets.Asset) (*assets.Asset, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	// First, validate that the source exists and belongs to this org.
	// Without this, a hostile caller could plant an asset under a
	// source_id that belongs to another org.
	src, err := s.GetKnowledgeSource(ctx, a.SourceID, a.OrgID)
	if err != nil {
		return nil, fmt.Errorf("knowledge_assets: source not found or foreign-org: %w", err)
	}
	_ = src // existence is what we care about; src not needed further

	// G8 SANITISATION: strip prompt-injection / jailbreak / hidden
	// markup BEFORE the asset reaches the index or the embedder.
	// Idempotent — already-clean text passes through unchanged.
	// Findings (what got stripped) are logged at the ingest boundary
	// but NOT persisted on the asset itself — the goal is "clean
	// data flows downstream", not "preserve the attack payload for
	// audit". A separate forensics audit table can be added later.
	if cleanTitle := security.Sanitize(a.Title); cleanTitle.Cleaned != "" {
		a.Title = cleanTitle.Cleaned
	}
	if cleanDesc := security.Sanitize(a.Description); cleanDesc.Cleaned != "" || a.Description == "" {
		a.Description = cleanDesc.Cleaned
	}

	tags := a.Tags
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, err := json.Marshal(assets.NormalizeTags(tags))
	if err != nil {
		return nil, err
	}
	payload := []byte(a.Payload)

	if a.ExternalID != "" {
		// Idempotent path. ON CONFLICT touches only ingestor fields;
		// state/pinned/boost/metrics remain whatever they were.
		_, err := s.ExecContext(ctx, `
			INSERT INTO knowledge_assets
				(org_id, source_id, external_id, type, title, description,
				 tags, payload, state, pinned, boost,
				 retrieval_count_30d, conversion_count_30d, last_retrieved_at,
				 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, NULL,
			        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT(org_id, source_id, external_id)
			WHERE external_id != ''
			DO UPDATE SET
				type        = excluded.type,
				title       = excluded.title,
				description = excluded.description,
				tags        = excluded.tags,
				payload     = excluded.payload,
				updated_at  = CURRENT_TIMESTAMP`,
			a.OrgID, a.SourceID, a.ExternalID, string(a.Type), a.Title, a.Description,
			string(tagsJSON), string(payload), string(a.State), boolToInt(a.Pinned), a.Boost,
		)
		if err != nil {
			return nil, err
		}
		// Hook: detect text content change → mark embedding pending.
		// Builds an asset clone with normalised tags so the hash matches
		// the SAME function the embedding worker will compute later.
		// Operator state (Pinned/Boost/State) is intentionally NOT part
		// of the hash — only Pinned/Boost/State Setters do not flow
		// through here, so this is correct by construction.
		s.markEmbeddingPendingIfTextChanged(ctx, a, assets.NormalizeTags(tags))
		// Re-read so we return the persisted state, including merged
		// CreatedAt and the actual UpdatedAt the DB stamped.
		return s.getKnowledgeAssetByExternalID(ctx, a.OrgID, a.SourceID, a.ExternalID)
	}
	// Insert-only path for assets without a stable ID. Caller is
	// responsible for deduping (typically via ContentFingerprint
	// stored as external_id, in which case this branch is dead code).
	id, err := s.InsertReturningID(ctx, `
		INSERT INTO knowledge_assets
			(org_id, source_id, external_id, type, title, description,
			 tags, payload, state, pinned, boost,
			 retrieval_count_30d, conversion_count_30d, last_retrieved_at,
			 created_at, updated_at)
		VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, NULL,
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id`,
		a.OrgID, a.SourceID, string(a.Type), a.Title, a.Description,
		string(tagsJSON), string(payload), string(a.State), boolToInt(a.Pinned), a.Boost,
	)
	if err != nil {
		return nil, err
	}
	// Same hook for the no-ExternalID insert path. The asset now has an
	// ID; reuse it for the hash UPDATE.
	a.ID = id
	s.markEmbeddingPendingIfTextChanged(ctx, a, assets.NormalizeTags(tags))
	return s.GetKnowledgeAsset(ctx, id, a.OrgID)
}

// SetKnowledgeAssetState is the operator-write for state transitions.
// Returns sql.ErrNoRows if the asset doesn't exist or belongs to a
// different org.
func (s *Store) SetKnowledgeAssetState(ctx context.Context, assetID, orgID int64, state assets.AssetState) error {
	if !state.IsKnown() {
		return errors.New("knowledge_assets: unknown state: " + string(state))
	}
	return s.updateAssetField(ctx, assetID, orgID, "state", string(state))
}

// SetKnowledgeAssetPinned is the operator-write for the pinned flag.
func (s *Store) SetKnowledgeAssetPinned(ctx context.Context, assetID, orgID int64, pinned bool) error {
	return s.updateAssetField(ctx, assetID, orgID, "pinned", boolToInt(pinned))
}

// SetKnowledgeAssetBoost is the operator-write for the boost slider.
// Boost is clamped to [0, 100] before persisting.
func (s *Store) SetKnowledgeAssetBoost(ctx context.Context, assetID, orgID int64, boost int) error {
	if boost < 0 {
		boost = 0
	}
	if boost > 100 {
		boost = 100
	}
	return s.updateAssetField(ctx, assetID, orgID, "boost", boost)
}

// IncrementKnowledgeAssetRetrieval is the L7 metric hook. Called from
// the retrieval engine after a Hit was used by the agent runtime.
func (s *Store) IncrementKnowledgeAssetRetrieval(ctx context.Context, assetID, orgID int64) error {
	if assetID <= 0 || orgID <= 0 {
		return fmt.Errorf("knowledge_assets: ids required")
	}
	res, err := s.ExecContext(ctx, `
		UPDATE knowledge_assets
		   SET retrieval_count_30d = retrieval_count_30d + 1,
		       last_retrieved_at   = CURRENT_TIMESTAMP
		 WHERE id = ? AND org_id = ?`, assetID, orgID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteKnowledgeAssetsForSource removes every asset that belongs to
// the given source. Used during source-disconnect and during test
// cleanup. Returns the number of rows deleted so callers can audit.
func (s *Store) DeleteKnowledgeAssetsForSource(ctx context.Context, sourceID, orgID int64) (int64, error) {
	if sourceID <= 0 || orgID <= 0 {
		return 0, fmt.Errorf("knowledge_assets: ids required")
	}
	res, err := s.ExecContext(ctx,
		`DELETE FROM knowledge_assets WHERE source_id = ? AND org_id = ?`,
		sourceID, orgID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// --- internals ---

const knowledgeAssetSelect = `
	SELECT id, org_id, source_id, external_id, type, title, description,
	       tags, payload, state, pinned, boost,
	       retrieval_count_30d, conversion_count_30d, last_retrieved_at,
	       created_at, updated_at
	FROM knowledge_assets`

func (s *Store) updateAssetField(ctx context.Context, assetID, orgID int64, column string, value any) error {
	if assetID <= 0 || orgID <= 0 {
		return fmt.Errorf("knowledge_assets: ids required")
	}
	// column is an internal allowlist constant from this package; never
	// user-supplied. Documented here so a future contributor doesn't try
	// to thread it from a handler.
	q := fmt.Sprintf(`UPDATE knowledge_assets
		SET %s = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND org_id = ?`, column)
	res, err := s.ExecContext(ctx, q, value, assetID, orgID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) getKnowledgeAssetByExternalID(ctx context.Context, orgID, sourceID int64, extID string) (*assets.Asset, error) {
	row := s.QueryRowContext(ctx, knowledgeAssetSelect+`
		WHERE org_id = ? AND source_id = ? AND external_id = ?`,
		orgID, sourceID, extID)
	return scanKnowledgeAsset(row)
}

func scanKnowledgeAsset(r scanRow) (*assets.Asset, error) {
	var (
		a            assets.Asset
		typ          string
		state        string
		pinned       int
		tagsRaw      string
		payloadRaw   string
		lastRetrRaw  sql.NullString
		createdAtRaw string
		updatedAtRaw string
	)
	if err := r.Scan(
		&a.ID, &a.OrgID, &a.SourceID, &a.ExternalID, &typ, &a.Title, &a.Description,
		&tagsRaw, &payloadRaw, &state, &pinned, &a.Boost,
		&a.Metrics.Retrievals30d, &a.Metrics.Conversions30d, &lastRetrRaw,
		&createdAtRaw, &updatedAtRaw,
	); err != nil {
		return nil, err
	}
	a.Type = assets.AssetType(typ)
	a.State = assets.AssetState(state)
	a.Pinned = pinned != 0
	a.Payload = json.RawMessage(payloadRaw)
	if err := json.Unmarshal([]byte(tagsRaw), &a.Tags); err != nil {
		a.Tags = []string{}
	}
	a.Metrics.LastRetrievedAt = optionalSQLiteTime(lastRetrRaw)
	a.CreatedAt = parseSQLiteTime(createdAtRaw)
	a.UpdatedAt = parseSQLiteTime(updatedAtRaw)
	return &a, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
